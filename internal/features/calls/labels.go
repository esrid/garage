package calls

import (
	"fmt"
	"strings"
	"time"

	"github.com/esrid/garage/internal/web/views"
)

// How a call reads on this page. It lives with the pages that show it.

// callerTitle names a call row: the customer when known, the number otherwise.
// Reuses the F04 rule so the dashboard and the history never disagree.
func callerTitle(call views.CallSummary) string {
	return views.ContactLabel(call.CustomerName, call.Phone)
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
