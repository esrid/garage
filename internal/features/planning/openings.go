package planning

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
)

// OpeningMutations lets the workshop declare when it is open.
//
// Until now ConfigureOpening was only reachable from tests, which meant a fresh
// installation had no opening at all — and with no stored opening the contract is
// explicit: no slot is ever offered. The assistant could not book anything, ever.
type OpeningMutations struct {
	service *appointment.Service
}

func NewOpeningMutations(service *appointment.Service) *OpeningMutations {
	return &OpeningMutations{service: service}
}

// Register mounts the opening declaration.
func (h *OpeningMutations) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /app/openings", h.Configure)
}

// Configure serves POST /app/openings. The form carries the civil day and two
// wall-clock times, because that is what a garage owner knows; the instants are
// built in the workshop's own timezone here.
func (h *OpeningMutations) Configure(w http.ResponseWriter, r *http.Request) {
	if err := parseAppointmentForm(w, r); err != nil {
		writeAppointmentError(w, r, err)
		return
	}

	// The day the operator was looking at, resolved through the backend so the
	// timezone comes from the tenant rather than from this process.
	day, err := h.service.Day(r.Context(), time.Now())
	if err != nil {
		writeAppointmentError(w, r, err)
		return
	}
	if raw := strings.TrimSpace(r.PostForm.Get("day")); raw != "" {
		parsed, parseErr := time.ParseInLocation(time.DateOnly, raw, day.Date.Location())
		if parseErr != nil {
			writeAppointmentError(w, r, openingValidation("day", "must be a YYYY-MM-DD date"))
			return
		}
		if day, err = h.service.Day(r.Context(), parsed.Add(12*time.Hour)); err != nil {
			writeAppointmentError(w, r, err)
			return
		}
	}

	start, err := openingInstant(day.Date, r.PostForm.Get("starts_at"), "starts_at")
	if err != nil {
		writeAppointmentError(w, r, err)
		return
	}
	end, err := openingInstant(day.Date, r.PostForm.Get("ends_at"), "ends_at")
	if err != nil {
		writeAppointmentError(w, r, err)
		return
	}
	capacity, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("capacity")))
	if err != nil {
		writeAppointmentError(w, r, openingValidation("capacity", "must be a number"))
		return
	}

	if _, err := h.service.ConfigureOpening(r.Context(), appointment.ConfigureOpeningInput{
		Start:    start,
		End:      end,
		Capacity: capacity,
	}); err != nil {
		writeAppointmentError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/planning?day="+day.Date.Format(time.DateOnly), http.StatusSeeOther)
}

// openingInstant turns "08:00" on a given day into an instant in that day's own
// location. Parsing the clock time as UTC and moving it later would shift the
// workshop's morning by four hours in Martinique.
func openingInstant(day time.Time, value, field string) (time.Time, error) {
	clock, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, openingValidation(field, "must be an HH:MM time")
	}
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, day.Location()), nil
}

func openingValidation(field, message string) error {
	return &domain.ValidationError{Entity: "opening", Errors: map[string]string{field: message}}
}
