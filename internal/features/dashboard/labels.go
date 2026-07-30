package dashboard

import (
	"strings"

	"github.com/esrid/garage/internal/web/views"
)

// The wording of a dashboard row. It lives with the page that shows it: no other
// feature titles a call by its caller or folds a task note into one line.

func callerLabel(call views.Call) string {
	return views.ContactLabel(call.CustomerName, call.Phone)
}
func taskLabel(task views.Task) string {
	return views.ContactLabel(task.CustomerName, task.Phone)
}
func callDetail(call views.Call) string {
	return joinDetail(views.OrDash(call.Subject), views.FormatDuration(call.Duration))
}

// The phone already titles the row when the name is unknown, so the detail line
// carries the note alone instead of repeating the number.
func taskDetail(task views.Task) string {
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
