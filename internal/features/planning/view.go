package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/web/views"
	"strconv"
	"strings"
	"time"
)

// The types below are what GET /app/planning renders. They are presentation
// DTOs, like the F04 ones above: the appointment domain must not depend on the
// UI, and no tenant ID ever reaches a view (PRD 7.1).
//
// views.Appointment is reused from the dashboard contract rather than cloned: the two
// pages show the same rows, with the same statuses and the same badge tones.

// Planning is one workshop day.
type Day struct {
	// Day is midnight in the tenant timezone, as resolved by the backend.
	Day time.Time
	// Timezone is the tenant's IANA name, shown so the person at the desk can
	// tell which clock these hours belong to.
	Timezone        string
	DurationMinutes int
	Openings        []Opening
	Appointments    []views.Appointment
	// Slots are the free ranges that fit DurationMinutes.
	Slots []Slot
	// RescheduleSlots holds the free ranges per appointment length in minutes. An
	// appointment is only ever offered slots its own duration fits into: offering
	// a slot the server would reject is a lie the desk pays for.
	RescheduleSlots map[int][]Slot
	// Degraded means the day could not be read. The page still renders, with the
	// reason, instead of a 500 or a blank screen.
	Degraded bool
	// SlotsUnavailable means the day was read but availability was not. The rows
	// are still worth showing; an empty slot list would read as "day full", which
	// is a different fact and one we do not know.
	SlotsUnavailable bool
	// Notices are the human-readable problems to show, in a fixed order.
	Notices []string
	// Alert is the closed error code from a failed mutation, as redirected by F02A
	// (amendment 2026-07-30). Separate from Notices: a notice explains why the page
	// shows less, an alert says an action the operator asked for did not happen.
	Alert string
}

// planningAlerts maps the closed set of mutation error codes to what the person at
// the desk needs to read. No message claims a cause the code does not carry: a
// conflict may be a taken slot or an already-recorded action, and pretending to
// know which is how a desk learns to distrust the screen.
var planningAlerts = map[string]string{
	"invalid":     "Demande refusée : créneau ou durée invalide. Rien n'a été modifié.",
	"not_found":   "Rendez-vous introuvable : il a peut-être été déplacé ou annulé entre-temps.",
	"conflict":    "Créneau indisponible, ou action déjà enregistrée. La liste ci-dessous est à jour.",
	"unavailable": "Le planning n'a pas pu être modifié. Réessayez dans un instant.",
}

// AlertMessage is the sentence to show, or "" when the visitor did not arrive from
// a failed mutation.
//
// The contract requires treating an unknown value as `unavailable`: a code we do
// not recognise means the backend and this page disagree, and the safe reading is
// "it may not have happened".
func (p Day) AlertMessage() string {
	code := strings.TrimSpace(p.Alert)
	if code == "" {
		return ""
	}
	if message, ok := planningAlerts[code]; ok {
		return message
	}
	return planningAlerts["unavailable"]
}

type Opening struct {
	Start, End time.Time
	Capacity   int
}

type Slot struct {
	Start, End time.Time
}

// PlanningDurations are the lengths the day filter offers. Every value is a
// multiple of 15 within the 15–480 range the F02A contract accepts.
var PlanningDurations = []int{15, 30, 45, 60, 90, 120, 180, 240}

// dayParam formats a day for the ?day= query parameter. The backend resolves it
// in the tenant timezone, so the wire format stays a plain civil date.
func dayParam(day time.Time) string {
	return day.Format(time.DateOnly)
}

// shiftDayParam moves the day by whole days. Arithmetic on the civil date, not
// on an instant: adding 24 hours across a DST boundary lands on the wrong day.
func shiftDayParam(day time.Time, days int) string {
	return dayParam(day.AddDate(0, 0, days))
}

func planningURL(day time.Time, durationMinutes int) string {
	return fmt.Sprintf("/app/planning?day=%s&duration_minutes=%d", dayParam(day), durationMinutes)
}

func hourLabel(instant time.Time) string {
	return instant.Format("15:04")
}

// rangeLabel reads as an hour range on one line: "08:00 – 09:00".
func rangeLabel(start, end time.Time) string {
	return hourLabel(start) + " – " + hourLabel(end)
}

func slotLabel(slot Slot) string {
	return rangeLabel(slot.Start, slot.End)
}

func openingLabel(opening Opening) string {
	return rangeLabel(opening.Start, opening.End)
}

// capacityLabel says how many vehicles the window takes at once. Plural is
// explicit: "1 véhicules" reads like a bug to the person at the desk.
func capacityLabel(capacity int) string {
	if capacity <= 1 {
		return strconv.Itoa(capacity) + " véhicule à la fois"
	}
	return strconv.Itoa(capacity) + " véhicules à la fois"
}

func minutesLabel(minutes int) string {
	if minutes < 60 {
		return strconv.Itoa(minutes) + " min"
	}
	if minutes%60 == 0 {
		return strconv.Itoa(minutes/60) + " h"
	}
	return fmt.Sprintf("%d h %02d", minutes/60, minutes%60)
}

func appointmentMinutes(item views.Appointment) int {
	return int(item.End.Sub(item.Start).Minutes())
}

// rowContext names the appointment a control acts on, for the accessible name.
//
// Two rows must never expose the same accessible name: tabbing through a day
// otherwise announces "Annuler le rendez-vous" three times with nothing to tell
// them apart, and someone cancels the wrong vehicle. Sighted users get the
// context from the row above; screen readers get it from a visually-hidden span.
func rowContext(item views.Appointment) string {
	who := strings.TrimSpace(item.CustomerName)
	if who == "" {
		who = "client inconnu"
	}
	return who + ", " + hourLabel(item.Start)
}

// planningActionable reports whether the desk may still move or cancel a row.
// The allowed transitions are frozen in docs/contracts/F02A-planning.md: only
// pending and confirmed lead to cancelled or to another time. Showing buttons
// for the other statuses would be showing buttons the server refuses.
func planningActionable(status string) bool {
	return status == "pending" || status == "confirmed"
}

// rescheduleOptions are the slots offered for one row: those matching its own
// length. Empty means the day is full for that length, and the view says so
// rather than rendering an empty select.
func rescheduleOptions(planning Day, item views.Appointment) []Slot {
	return planning.RescheduleSlots[appointmentMinutes(item)]
}

// idempotencyKey derives the opaque key the F02A mutation forms require.
//
// Deterministic on purpose: the same rendered form submitted twice — a
// double-click, a refresh, a flaky connection — carries the same key, and the
// backend replays its first answer instead of moving the appointment twice.
//
// The row's UpdatedAt is what makes the key fresh, and it has to be a marker the
// server bumps on every write. Keying on the start time instead looks equivalent
// and is not: an appointment moved 09:00 → 10:00 → 09:00 would land back on a key
// already spent, and every later move from 09:00 would answer 409 for as long as
// it sat there — a legitimate action blocked by a stale replay.
//
// A stale form submitted from the browser's back button still carries the old
// UpdatedAt with new data, which the contract turns into a 409 instead of a
// silent double booking. That is the safe direction.
func idempotencyKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}

func rowState(item views.Appointment) string {
	return item.ID + "@" + item.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func rescheduleKey(item views.Appointment) string {
	return idempotencyKey("reschedule", rowState(item), strconv.Itoa(appointmentMinutes(item)))
}

func cancelKey(item views.Appointment) string {
	return idempotencyKey("cancel", rowState(item))
}

// StatusMove is a button the desk may press on a row: the target status and what
// it is called in French.
type StatusMove struct {
	Status string
	Label  string
}

var statusMoveLabels = map[string]string{
	"confirmed":   "Confirmer",
	"in_progress": "Démarrer",
	"done":        "Terminer",
	"no_show":     "Client absent",
	"cancelled":   "Annuler le rendez-vous",
}

// statusMoves lists what this row allows, straight from the domain's frozen
// transition table. The buttons cannot drift from what the service accepts,
// because they are the same list.
//
// Cancelling is not here: it has its own form, and mixing a terminal action into
// the same row of buttons is how one gets pressed by accident.
func statusMoves(item views.Appointment) []StatusMove {
	moves := make([]StatusMove, 0, 3)
	for _, status := range appointment.NextStatuses(appointment.Status(item.Status)) {
		if status == appointment.StatusCancelled {
			continue
		}
		moves = append(moves, StatusMove{Status: string(status), Label: statusMoveLabels[string(status)]})
	}
	return moves
}

// statusKey derives the idempotency-free form key for a status move: the same
// deterministic rule as the other row forms, so a double submit is a no-op.
func statusKey(item views.Appointment, status string) string {
	return idempotencyKey("status", rowState(item), status)
}
