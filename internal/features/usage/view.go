package usage

import (
	"fmt"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
)

// View is what the page renders: the month's consumption, plus anything that
// prevented it from being read.
type View struct {
	Usage    conversation.Usage
	Degraded bool
	Notices  []string
}

// AlertMessage states the threshold crossed, in the workshop's terms. Silence
// below 70 %: a bar that shouts at 12 % teaches people to ignore it.
func (v View) AlertMessage() string {
	// The threshold decides whether to speak; the sentence states the real number.
	// Announcing "70 %" on a page showing 83 % is how a meter loses its reader.
	switch v.Usage.Alert() {
	case conversation.AlertOverAt:
		return "Quota mensuel atteint. Les appels continuent : le dépassement est facturé ou vous fait monter d'offre."
	case conversation.AlertWarningAt, conversation.AlertNoticeAt:
		return fmt.Sprintf("%d %% du quota mensuel consommé.", v.Usage.Percent())
	default:
		return ""
	}
}

// AlertTone maps the threshold to a badge tone, so the colour follows the fact.
func (v View) AlertTone() string {
	switch v.Usage.Alert() {
	case conversation.AlertOverAt:
		return "notice-alert"
	case conversation.AlertWarningAt, conversation.AlertNoticeAt:
		return ""
	default:
		return ""
	}
}

func (v View) MonthLabel() string {
	return frenchMonth(v.Usage.Month)
}

func (v View) MonthParam() string {
	return v.Usage.Month.Format("2006-01")
}

func (v View) ShiftMonth(months int) string {
	return v.Usage.Month.AddDate(0, months, 0).Format("2006-01")
}

// PercentWidth caps the bar at 100 while the number above it keeps the truth: a
// bar cannot be 130 % long, but the workshop still has to read 130 %.
func (v View) PercentWidth() int {
	return min(v.Usage.Percent(), 100)
}

var frenchMonths = [...]string{
	"janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre",
}

func frenchMonth(month time.Time) string {
	return fmt.Sprintf("%s %d", frenchMonths[int(month.Month())-1], month.Year())
}
