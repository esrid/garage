package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/web/views"
)

// PlanningReader is the read side of the F02A scheduling service, and all F02B
// consumes. The three mutations stay with AppointmentMutations, as the frozen
// contract assigns them.
//
// No tenant ID in the signatures: it travels in ctx, put there by the tenant
// middleware, so no frontend caller can pass one (PRD 7.1).
type PlanningReader interface {
	Day(ctx context.Context, day time.Time) (appointment.Day, error)
	AvailableSlots(ctx context.Context, query appointment.AvailabilityQuery) ([]appointment.Slot, error)
}

const (
	defaultPlanningMinutes = 60
	// maxRescheduleLookups bounds the availability queries one page may trigger:
	// one per distinct appointment length. A day with more shapes than this shows
	// its rows without move options rather than hammering the database.
	maxRescheduleLookups = 6
)

// Planning renders the workshop day: opening windows, free slots and the
// appointments, with the move and cancel forms that post to F02A.
type Planning struct {
	reader PlanningReader
	// now is injected so a rendered day is a function of input, not of the wall
	// clock, which keeps these handlers testable.
	now func() time.Time
}

func NewPlanning(reader PlanningReader) *Planning {
	return &Planning{reader: reader, now: time.Now}
}

// Page serves GET /app/planning.
func (h *Planning) Page(w http.ResponseWriter, r *http.Request) {
	data, status := h.load(r)
	h.render(w, r, status, views.PlanningPage(data))
}

// Fragment serves GET /app/planning/day: the day block alone, for the duration
// filter's htmx swap.
func (h *Planning) Fragment(w http.ResponseWriter, r *http.Request) {
	data, status := h.load(r)
	h.render(w, r, status, views.PlanningDay(data))
}

// load builds the view data and the status to answer with.
//
// Status policy, deliberately narrow: 401 when there is no tenant, because there
// is nothing to show and htmx must not swap that in; 200 in every other case,
// including an unreadable date or an unavailable database. These are HTML pages
// for a human at a desk: they carry the reason in a notice, and the failure is in
// the structured log for whoever watches the service.
func (h *Planning) load(r *http.Request) (views.Planning, int) {
	ctx := r.Context()
	query := r.URL.Query()
	minutes, notices := planningMinutes(query.Get("duration_minutes"))

	// The requested civil date is meaningless without the tenant timezone, and
	// that timezone lives in the database. So: ask for the current day first, then
	// read the requested date in the location it reports. Parsing "2026-07-30" as
	// UTC and using it directly would ask for 2026-07-29 20:00 in Martinique —
	// the previous day (coordination note in WORKBOARD.md).
	day, err := h.reader.Day(ctx, h.now())
	if err != nil {
		return h.unavailable(ctx, err, minutes)
	}
	if raw := strings.TrimSpace(query.Get("day")); raw != "" {
		requested, parseErr := time.ParseInLocation(time.DateOnly, raw, day.Date.Location())
		if parseErr != nil {
			notices = append(notices, "Date illisible : voici la journée en cours.")
		} else {
			// Midday of the requested date: the backend resolves the day around this
			// instant, and midnight would sit on the boundary a timezone shift moves.
			day, err = h.reader.Day(ctx, requested.Add(12*time.Hour))
			if err != nil {
				return h.unavailable(ctx, err, minutes)
			}
		}
	}

	data := views.Planning{
		Day:             day.Date,
		Timezone:        day.Timezone,
		DurationMinutes: minutes,
		Openings:        make([]views.Opening, 0, len(day.Openings)),
		Appointments:    make([]views.Appointment, 0, len(day.Appointments)),
		Notices:         notices,
	}
	for _, opening := range day.Openings {
		data.Openings = append(data.Openings, views.Opening{
			// Explicitly in the tenant location: the desk reads workshop hours, not
			// whatever zone the driver handed us the timestamp in.
			Start:    opening.Start.In(day.Date.Location()),
			End:      opening.End.In(day.Date.Location()),
			Capacity: opening.Capacity,
		})
	}
	for _, entry := range day.Appointments {
		data.Appointments = append(data.Appointments, views.Appointment{
			ID:           entry.ID,
			Start:        entry.Start.In(day.Date.Location()),
			End:          entry.End.In(day.Date.Location()),
			CustomerName: entry.CustomerName,
			Vehicle:      entry.VehicleLabel,
			Plate:        entry.Plate,
			Service:      entry.ServiceLabel,
			Status:       string(entry.Status),
		})
	}

	// Availability failing alone does not cost the day: the rows the desk needs are
	// already read, and losing them too would be a worse page than an honest
	// "slots unavailable".
	slots, err := h.slots(ctx, day, minutes)
	if err != nil {
		slog.ErrorContext(ctx, "planning: slots unavailable", "minutes", minutes, "err", err)
		data.SlotsUnavailable = true
		data.Notices = append(data.Notices, "Les créneaux libres sont momentanément indisponibles.")
		return data, http.StatusOK
	}
	data.Slots = slots
	data.RescheduleSlots = h.rescheduleSlots(ctx, day, data, minutes, slots)
	return data, http.StatusOK
}

// slots asks for the free ranges of one length on this day.
func (h *Planning) slots(ctx context.Context, day appointment.Day, minutes int) ([]views.Slot, error) {
	found, err := h.reader.AvailableSlots(ctx, appointment.AvailabilityQuery{
		Day:      day.Date,
		Duration: time.Duration(minutes) * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	slots := make([]views.Slot, 0, len(found))
	for _, slot := range found {
		slots = append(slots, views.Slot{
			Start: slot.Start.In(day.Date.Location()),
			End:   slot.End.In(day.Date.Location()),
		})
	}
	return slots, nil
}

// rescheduleSlots collects the options each movable row may be offered: the free
// ranges matching its own length, so no row is ever shown a slot the server
// would refuse.
//
// One query per distinct length, bounded, and the page duration is reused when it
// matches. A failure here degrades to "no other slot today" on the affected rows
// instead of failing the whole page: the day itself is still worth reading.
func (h *Planning) rescheduleSlots(ctx context.Context, day appointment.Day, data views.Planning, pageMinutes int, pageSlots []views.Slot) map[int][]views.Slot {
	lengths := make([]int, 0, len(data.Appointments))
	for _, item := range data.Appointments {
		minutes := int(item.End.Sub(item.Start).Minutes())
		// A non-positive length is corrupt data, not a duration to look slots up
		// for: the row still renders, without move options.
		if minutes <= 0 {
			continue
		}
		if !slices.Contains(lengths, minutes) {
			lengths = append(lengths, minutes)
		}
	}
	slices.Sort(lengths)
	if len(lengths) > maxRescheduleLookups {
		slog.WarnContext(ctx, "planning: too many appointment lengths for slot lookup", "lengths", len(lengths))
		return nil
	}

	options := make(map[int][]views.Slot, len(lengths))
	for _, minutes := range lengths {
		if minutes == pageMinutes {
			options[minutes] = pageSlots
			continue
		}
		found, err := h.slots(ctx, day, minutes)
		if err != nil {
			slog.ErrorContext(ctx, "planning: slots unavailable", "minutes", minutes, "err", err)
			continue
		}
		options[minutes] = found
	}
	return options
}

// planningMinutes reads the duration filter. An unusable value falls back to one
// hour and says so: silently rendering a different length than the one asked for
// is how a desk stops trusting the page.
func planningMinutes(raw string) (int, []string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultPlanningMinutes, nil
	}
	minutes, err := strconv.Atoi(value)
	if err != nil || !slices.Contains(views.PlanningDurations, minutes) {
		return defaultPlanningMinutes, []string{"Durée non proposée : affichage en 1 h."}
	}
	return minutes, nil
}

func (h *Planning) unavailable(ctx context.Context, err error, minutes int) (views.Planning, int) {
	var unauthorized *domain.UnauthorizedError
	if errors.As(err, &unauthorized) {
		return views.Planning{
			DurationMinutes: minutes,
			Degraded:        true,
			Notices:         []string{"Connexion requise pour afficher le planning."},
		}, http.StatusUnauthorized
	}
	slog.ErrorContext(ctx, "planning: day unavailable", "err", err)
	return views.Planning{
		DurationMinutes: minutes,
		Degraded:        true,
		Notices:         []string{"Le planning est momentanément indisponible. Réessayez dans un instant."},
	}, http.StatusOK
}

func (h *Planning) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	// Operational data: a cached copy shown after a back navigation would tell the
	// desk a slot is free when it was taken ten minutes ago.
	w.Header().Set("Cache-Control", "no-store")
	// templ.Handler buffers, so a mid-render error cannot emit half a page under a
	// 200.
	templ.Handler(component, templ.WithStatus(status)).ServeHTTP(w, r)
}
