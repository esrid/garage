package views

import (
	"fmt"
	"strings"
	"time"
)

// The call history DTOs, frozen in docs/contracts/F15-call-history.md. They are
// presentation types: the conversation domain never imports a view, and the
// adapter Agent A writes maps provider JSON into these.

// CallHistory is the calls of one workshop day.
type CallHistory struct {
	// Day is midnight in the tenant timezone.
	Day      time.Time
	Timezone string
	Calls    []CallSummary
	// Degraded means the day could not be read; the page says so instead of
	// showing an empty list, which would read as "no calls today".
	Degraded bool
	Notices  []string
}

type CallSummary struct {
	ID           string
	At           time.Time
	Duration     time.Duration
	CustomerName string
	Phone        string
	Outcome      string
	Status       string
	Summary      string
}

// CallDetail is one call with its transcript.
type CallDetail struct {
	CallSummary
	Turns []CallTurn
}

type CallTurn struct {
	Role string
	Text string
	// At is the offset from the start of the call, zero when the provider gave
	// none. Rendered only when non-zero: a fake "00:00" on every line is noise.
	At time.Duration
}

// callerTitle names a call row: the customer when known, the number otherwise.
// Reuses the F04 rule so the dashboard and the history never disagree.
func callerTitle(call CallSummary) string {
	return contactLabel(call.CustomerName, call.Phone)
}

// turnRole is the speaker label. Provider roles are free strings: an unknown one
// keeps its raw value rather than being folded into "agent" or "client".
func turnRole(role string) string {
	switch strings.TrimSpace(role) {
	case "agent", "assistant":
		return "Assistant"
	case "user", "customer":
		return "Client"
	case "":
		return "—"
	default:
		return role
	}
}

// turnOffset formats the position of a turn inside the call, "" when unknown.
func turnOffset(at time.Duration) string {
	if at <= 0 {
		return ""
	}
	seconds := int(at.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

// callDayParam formats the day for the ?day= parameter, like the planning page.
func callDayParam(day time.Time) string {
	return day.Format(time.DateOnly)
}
