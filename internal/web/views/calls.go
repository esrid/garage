package views

import (
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
