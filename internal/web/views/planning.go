package views

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The types below are what GET /app/planning renders. They are presentation
// DTOs, like the F04 ones above: the appointment domain must not depend on the
// UI, and no tenant ID ever reaches a view (PRD 7.1).
//
// Appointment is reused from the dashboard contract rather than cloned: the two
// pages show the same rows, with the same statuses and the same badge tones.

// Planning is one workshop day.
type Planning struct {
	// Day is midnight in the tenant timezone, as resolved by the backend.
	Day time.Time
	// Timezone is the tenant's IANA name, shown so the person at the desk can
	// tell which clock these hours belong to.
	Timezone        string
	DurationMinutes int
	Openings        []Opening
	Appointments    []Appointment
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

func appointmentMinutes(item Appointment) int {
	return int(item.End.Sub(item.Start).Minutes())
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
func rescheduleOptions(planning Planning, item Appointment) []Slot {
	return planning.RescheduleSlots[appointmentMinutes(item)]
}

// idempotencyKey derives the opaque key the F02A mutation forms require.
//
// Deterministic on purpose: the same rendered form submitted twice — a
// double-click, a refresh, a flaky connection — carries the same key, and the
// backend replays its first answer instead of moving the appointment twice.
//
// The current start time is part of the key, so once a move succeeds the next
// render produces a different key and the following move is a new request. Going
// back in the browser and submitting the stale form again reuses the key with
// different data, which the contract turns into a 409 rather than a silent
// double booking. That is the safe direction.
func idempotencyKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}

func rescheduleKey(item Appointment) string {
	return idempotencyKey("reschedule", item.ID, item.Start.Format(time.RFC3339), strconv.Itoa(appointmentMinutes(item)))
}

func cancelKey(item Appointment) string {
	return idempotencyKey("cancel", item.ID, item.Start.Format(time.RFC3339))
}
