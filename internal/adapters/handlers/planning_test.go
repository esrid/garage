package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
)

// Martinique as a fixed zone rather than time.LoadLocation: the tests must not
// depend on the tzdata of the machine running them, and the island has no DST.
var martinique = time.FixedZone("AST", -4*60*60)

var planningNow = time.Date(2026, 7, 30, 9, 30, 0, 0, martinique)

// planningStub answers like the PostgreSQL reader: Day resolves the requested
// instant to midnight in the tenant timezone, and slots are per requested length.
type planningStub struct {
	openings     []appointment.Opening
	appointments []appointment.DayEntry
	slots        map[int][]appointment.Slot
	dayErr       error
	slotsErr     error

	dayCalls  []time.Time
	slotCalls []int
}

func (s *planningStub) Day(_ context.Context, day time.Time) (appointment.Day, error) {
	s.dayCalls = append(s.dayCalls, day)
	if s.dayErr != nil {
		return appointment.Day{}, s.dayErr
	}
	local := day.In(martinique)
	return appointment.Day{
		Date:         time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, martinique),
		Timezone:     "America/Martinique",
		Openings:     s.openings,
		Appointments: s.appointments,
	}, nil
}

func (s *planningStub) AvailableSlots(_ context.Context, query appointment.AvailabilityQuery) ([]appointment.Slot, error) {
	minutes := int(query.Duration.Minutes())
	s.slotCalls = append(s.slotCalls, minutes)
	if s.slotsErr != nil {
		return nil, s.slotsErr
	}
	return s.slots[minutes], nil
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 7, 30, hour, minute, 0, 0, martinique)
}

func fullPlanningStub() *planningStub {
	return &planningStub{
		openings: []appointment.Opening{{ID: "op-1", Start: at(8, 0), End: at(12, 0), Capacity: 2}},
		appointments: []appointment.DayEntry{
			{
				Appointment: appointment.Appointment{
					ID: "rdv-1", Start: at(9, 0), End: at(10, 0),
					ServiceLabel: "Vidange", Status: appointment.StatusConfirmed,
				},
				CustomerName: "Marie Lubin", VehicleLabel: "Clio IV", Plate: "AB-123-CD",
			},
			{
				Appointment: appointment.Appointment{
					ID: "rdv-2", Start: at(10, 30), End: at(11, 0),
					ServiceLabel: "Diagnostic", Status: appointment.StatusPending,
				},
				CustomerName: "Jean-Claude Sainte-Rose", VehicleLabel: "Hilux",
			},
			{
				Appointment: appointment.Appointment{
					ID: "rdv-3", Start: at(11, 0), End: at(12, 0),
					ServiceLabel: "Révision", Status: appointment.StatusCancelled,
				},
				CustomerName: "Garage Morne-Rouge",
			},
		},
		slots: map[int][]appointment.Slot{
			60: {{Start: at(8, 0), End: at(9, 0)}, {Start: at(11, 0), End: at(12, 0)}},
			30: {{Start: at(8, 0), End: at(8, 30)}, {Start: at(8, 30), End: at(9, 0)}},
		},
	}
}

func newTestPlanning(reader PlanningReader) *Planning {
	h := NewPlanning(reader)
	h.now = func() time.Time { return planningNow }
	return h
}

func getPlanning(t *testing.T, handler http.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func TestPlanningPageRendersTheDay(t *testing.T) {
	response := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()

	for _, want := range []string{
		"jeudi 30 juillet 2026",
		"America/Martinique",
		"08:00 – 12:00",      // opening
		"2 véhicules",        // capacity, plural
		"08:00 – 09:00",      // free slot for the default hour
		"Marie Lubin",        // appointment rows
		"AB-123-CD",
		"Vidange",
		"Confirmé",
		"Annulé",
		`id="planning-day"`, // the fragment root the htmx swap targets
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	// Hours must be the tenant's, not UTC: 09:00 in Martinique is 13:00 UTC.
	if strings.Contains(body, "13:00") {
		t.Error("page renders UTC hours instead of the tenant timezone")
	}
}

// The trap Agent A flagged: a civil date parsed as UTC and used as an instant
// lands on the previous day in Martinique.
func TestPlanningDayParameterIsReadInTheTenantTimezone(t *testing.T) {
	stub := fullPlanningStub()
	getPlanning(t, newTestPlanning(stub).Page, "/app/planning?day=2026-07-31")

	if len(stub.dayCalls) != 2 {
		t.Fatalf("Day called %d times, want 2 (current day, then the requested one)", len(stub.dayCalls))
	}
	requested := stub.dayCalls[1].In(martinique)
	if got := requested.Format(time.DateOnly); got != "2026-07-31" {
		t.Errorf("asked the backend for %s, want 2026-07-31", got)
	}
}

func TestPlanningWithoutDayParameterAsksOnlyForToday(t *testing.T) {
	stub := fullPlanningStub()
	body := getPlanning(t, newTestPlanning(stub).Page, "/app/planning").Body.String()

	if len(stub.dayCalls) != 1 {
		t.Errorf("Day called %d times, want 1", len(stub.dayCalls))
	}
	if !strings.Contains(body, "jeudi 30 juillet 2026") {
		t.Error("page does not show the current day")
	}
}

func TestPlanningKeepsRenderingOnAnUnreadableDate(t *testing.T) {
	response := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning?day=31-07-2026")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: an unreadable date must not blank the page", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Date illisible") {
		t.Error("the page does not say the date was unreadable")
	}
	if !strings.Contains(body, "jeudi 30 juillet 2026") {
		t.Error("the page should fall back to the current day")
	}
}

func TestPlanningFallsBackOnAnUnofferedDuration(t *testing.T) {
	stub := fullPlanningStub()
	body := getPlanning(t, newTestPlanning(stub).Page, "/app/planning?duration_minutes=37").Body.String()

	if !strings.Contains(body, "Durée non proposée") {
		t.Error("the page does not say the duration was refused")
	}
	if len(stub.slotCalls) == 0 || stub.slotCalls[0] != 60 {
		t.Errorf("slot lookups = %v, want the first one at 60 minutes", stub.slotCalls)
	}
}

func TestPlanningAsksForTheSelectedDuration(t *testing.T) {
	stub := fullPlanningStub()
	body := getPlanning(t, newTestPlanning(stub).Page, "/app/planning?duration_minutes=30").Body.String()

	if len(stub.slotCalls) == 0 || stub.slotCalls[0] != 30 {
		t.Errorf("slot lookups = %v, want the first one at 30 minutes", stub.slotCalls)
	}
	if !strings.Contains(body, `value="30"`) || !strings.Contains(body, "selected") {
		t.Error("the duration filter does not keep the selected value")
	}
}

// One availability query per distinct appointment length, and the page duration
// is reused instead of asked twice.
func TestPlanningLooksSlotsUpOncePerAppointmentLength(t *testing.T) {
	stub := fullPlanningStub()
	getPlanning(t, newTestPlanning(stub).Page, "/app/planning?duration_minutes=60")

	if len(stub.slotCalls) != 2 {
		t.Fatalf("slot lookups = %v, want two: 60 for the page, 30 for the pending row", stub.slotCalls)
	}
	if stub.slotCalls[0] != 60 || stub.slotCalls[1] != 30 {
		t.Errorf("slot lookups = %v, want [60 30]", stub.slotCalls)
	}
}

func TestPlanningOffersOnlySlotsThatFitTheAppointment(t *testing.T) {
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning?duration_minutes=60").Body.String()

	// rdv-2 lasts 30 minutes: its select must offer the 30-minute ranges.
	form := formFor(t, body, "/app/appointments/rdv-2/reschedule")
	if !strings.Contains(form, `value="2026-07-30T08:30:00-04:00"`) {
		t.Error("the 30-minute row is not offered the 08:30 slot")
	}
	if !strings.Contains(form, `name="duration_minutes" value="30"`) {
		t.Error("the row does not keep its own length")
	}
	// A 60-minute range would not fit a 30-minute grid slot list; the row must not
	// offer one the server would then have to refuse.
	if strings.Contains(form, `value="2026-07-30T11:00:00-04:00"`) {
		t.Error("the 30-minute row is offered a slot from the page duration")
	}
}

func TestPlanningHidesActionsOnTerminalAppointments(t *testing.T) {
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String()

	if strings.Contains(body, "/app/appointments/rdv-3/cancel") {
		t.Error("a cancelled appointment must not offer a cancel form")
	}
	if !strings.Contains(body, "/app/appointments/rdv-1/cancel") {
		t.Error("a confirmed appointment must offer a cancel form")
	}
}

// The mutation forms need an opaque idempotency key. It must be stable for a
// given appointment state, so a double submit replays instead of moving twice.
func TestPlanningIdempotencyKeysAreStableAndDistinct(t *testing.T) {
	keys := regexp.MustCompile(`name="idempotency_key" value="([0-9a-f]+)"`)

	first := keys.FindAllStringSubmatch(getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String(), -1)
	second := keys.FindAllStringSubmatch(getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String(), -1)

	if len(first) < 4 {
		t.Fatalf("found %d idempotency keys, want one per move and cancel form", len(first))
	}
	seen := make(map[string]bool, len(first))
	for i, match := range first {
		if match[1] != second[i][1] {
			t.Errorf("key %d changed between two identical renders", i)
		}
		if seen[match[1]] {
			t.Errorf("key %q is reused by two different forms", match[1])
		}
		seen[match[1]] = true
	}
}

func TestPlanningInventsNoOpeningHours(t *testing.T) {
	stub := fullPlanningStub()
	stub.openings = nil
	stub.slots = nil

	body := getPlanning(t, newTestPlanning(stub).Page, "/app/planning").Body.String()
	if !strings.Contains(body, "Aucune ouverture enregistrée") {
		t.Error("the page does not say the day has no stored opening")
	}
	for _, forbidden := range []string{"08:00 – 09:00", "08:00 – 17:00"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the page invented the slot %q", forbidden)
		}
	}
}

func TestPlanningRequiresATenant(t *testing.T) {
	stub := fullPlanningStub()
	stub.dayErr = &domain.UnauthorizedError{Message: "tenant context required"}

	response := getPlanning(t, newTestPlanning(stub).Page, "/app/planning")
	if response.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Connexion requise") {
		t.Error("the page does not ask for authentication")
	}
}

// A database outage degrades the page instead of blanking it: the person at the
// desk gets a reason, and the failure is in the log.
func TestPlanningDegradesWhenTheBackendIsDown(t *testing.T) {
	stub := fullPlanningStub()
	stub.dayErr = errors.New("database is down")

	response := getPlanning(t, newTestPlanning(stub).Page, "/app/planning")
	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "momentanément indisponible") {
		t.Error("the page does not explain why it is empty")
	}
	if strings.Contains(body, "database is down") {
		t.Error("the page leaks the backend error")
	}
}

// Slots failing alone must not cost the whole day: the rows still render.
func TestPlanningKeepsTheDayWhenSlotsFail(t *testing.T) {
	stub := fullPlanningStub()
	stub.slotsErr = errors.New("slot query failed")

	response := getPlanning(t, newTestPlanning(stub).Page, "/app/planning")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "créneaux libres sont momentanément indisponibles") {
		t.Error("the page does not explain the missing slots")
	}
	if !strings.Contains(body, "Marie Lubin") {
		t.Error("the appointments must still render when only availability failed")
	}
	// "Journée complète" would state a fact we do not have.
	if strings.Contains(body, "Journée complète") {
		t.Error("the page claims the day is full when availability is unknown")
	}
}

func TestPlanningFragmentIsSwappable(t *testing.T) {
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Fragment, "/app/planning/day?duration_minutes=30").Body.String()

	if strings.Contains(body, "<html") || strings.Contains(body, "app-header") {
		t.Error("the fragment must not carry the page shell")
	}
	if !strings.HasPrefix(strings.TrimSpace(body), `<div class="planning-day" id="planning-day"`) {
		t.Errorf("the fragment root is not the htmx target: %.80s", body)
	}
}

// formFor returns the markup of the form posting to action, so a test can assert
// on one row instead of on the whole page.
func formFor(t *testing.T, body, action string) string {
	t.Helper()
	start := strings.Index(body, `action="`+action+`"`)
	if start < 0 {
		t.Fatalf("no form posting to %s", action)
	}
	end := strings.Index(body[start:], "</form>")
	if end < 0 {
		t.Fatalf("form posting to %s is not closed", action)
	}
	return body[start : start+end]
}

// TestWritePlanningPreview dumps the rendered page so it can be opened in a
// browser and looked at. Skipped unless PLANNING_PREVIEW names a file: a test
// suite does not write to disk by default.
func TestWritePlanningPreview(t *testing.T) {
	path := os.Getenv("PLANNING_PREVIEW")
	if path == "" {
		t.Skip("set PLANNING_PREVIEW=<file> to dump the planning page")
	}
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.Bytes()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write preview: %v", err)
	}
}
