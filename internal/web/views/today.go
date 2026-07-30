package views

import (
	"fmt"
	"strings"
	"time"
)

// The types below are the GET /app data contract, frozen in
// docs/contracts/F04-dashboard-today.md. They are presentation DTOs on purpose:
// the domain must not depend on the UI.

// Today is everything the dashboard renders for one calendar day.
type Today struct {
	Day          time.Time
	Calls        []Call
	Appointments []Appointment
	Tasks        []Task
}

type Call struct {
	ID           string
	At           time.Time
	Duration     time.Duration
	CustomerName string // empty when the caller is unknown
	Phone        string
	Subject      string
	Outcome      string
	Transferred  bool
}

type Appointment struct {
	ID           string
	Start, End   time.Time
	CustomerName string
	Vehicle      string
	Plate        string // may be empty: never invent one
	Service      string
	Status       string
	// UpdatedAt is the server's marker of this row's state. The planning page
	// derives its idempotency keys from it, so any write invalidates the forms
	// rendered before it. Amendment 2026-07-30 to the F04 contract: additive, and
	// the dashboard does not read it.
	UpdatedAt time.Time
}

type Task struct {
	ID           string
	CreatedAt    time.Time
	Kind         string
	CustomerName string
	Phone        string
	Note         string
}

// badgeTone maps a backend status to a CSS tone. Tones, not domain vocabulary,
// so a new status upstream needs no CSS change.
//
// An unrecognised status returns the neutral tone and keeps its raw text: an
// unknown value is a visible integration bug, not something to paper over.
func badgeTone(status string) string {
	switch status {
	case "booked", "confirmed", "done":
		return "badge-success"
	case "pending", "in_progress", "rescheduled", "transferred":
		return "badge-warning"
	case "cancelled", "no_show", "dropped":
		return "badge-danger"
	case "callback", "quote", "info":
		return "badge-info"
	default:
		return ""
	}
}

var statusLabels = map[string]string{
	// Call.Outcome
	"booked":      "RDV pris",
	"rescheduled": "Déplacé",
	"callback":    "Rappel",
	"quote":       "Devis",
	"info":        "Info",
	"transferred": "Transféré",
	"dropped":     "Abandonné",
	// Appointment.Status
	"pending":     "En attente",
	"confirmed":   "Confirmé",
	"in_progress": "En cours",
	"done":        "Terminé",
	"no_show":     "Absent",
	// Shared by both
	"cancelled": "Annulé",
}

func statusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}

// orDash renders missing data as an em dash. Prices, plates and statuses are
// never invented to fill a gap (PRD 7.1).
func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

// contactLabel names the person a row is about: the customer name when known,
// otherwise the number they called from. A row titled "—" while we hold their
// number is useless to the person at the desk, so both calls and tasks use this.
func contactLabel(name, phone string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if strings.TrimSpace(phone) != "" {
		return phone
	}
	return "Numéro inconnu"
}

func callerLabel(call Call) string {
	return contactLabel(call.CustomerName, call.Phone)
}

func taskLabel(task Task) string {
	return contactLabel(task.CustomerName, task.Phone)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	seconds := int(d.Round(time.Second).Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%d s", seconds)
	}
	return fmt.Sprintf("%d min %02d s", seconds/60, seconds%60)
}

func callDetail(call Call) string {
	return joinDetail(orDash(call.Subject), formatDuration(call.Duration))
}

// The phone already titles the row when the name is unknown, so the detail line
// carries the note alone instead of repeating the number.
func taskDetail(task Task) string {
	return joinDetail(strings.TrimSpace(task.Note))
}

func joinDetail(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "—" {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return "—"
	}
	return strings.Join(kept, " — ")
}

var (
	frenchDays = [...]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
	// Go's time package carries no locale, so the French names live here.
	frenchMonths = [...]string{
		"janvier", "février", "mars", "avril", "mai", "juin",
		"juillet", "août", "septembre", "octobre", "novembre", "décembre",
	}
)

func frenchDate(day time.Time) string {
	return fmt.Sprintf("%s %d %s %d",
		frenchDays[int(day.Weekday())], day.Day(), frenchMonths[int(day.Month())-1], day.Year())
}
