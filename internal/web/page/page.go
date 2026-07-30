package page

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
)

// ErrNoDayParameter says the request carried no ?day=, so the caller keeps the
// day the backend already returned.
var ErrNoDayParameter = errors.New("handlers: no day parameter")

// DayUnreadableNotice is what every day-scoped page says about a date it could
// not read. One sentence, so two pages cannot phrase the same problem
// differently.
const DayUnreadableNotice = "Date illisible : voici la journée en cours."

// requestedDay turns a ?day=YYYY-MM-DD parameter into the instant to ask the
// backend for.
//
// The rule lives here because it is the trap this codebase keeps meeting: a
// civil date means nothing without the workshop's timezone, so it is parsed
// inside the location the backend just reported, never in UTC — "2026-07-30"
// read as UTC and used as an instant asks for the 29th in Martinique.
//
// The instant aims at midday: the backend resolves a day around it, and midnight
// sits exactly on the boundary a timezone shift moves.
func RequestedDay(raw string, location *time.Location) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, ErrNoDayParameter
	}
	parsed, err := time.ParseInLocation(time.DateOnly, value, location)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.Add(12 * time.Hour), nil
}

// renderPage writes an operational page: never cached, and buffered by templ so
// a mid-render failure cannot emit half a page under a 200.
//
// The dashboard, the planning and the call history each had their own copy of
// these three lines; the guarantee is easier to keep when it exists once.
func Render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Cache-Control", "no-store")
	templ.Handler(component, templ.WithStatus(status)).ServeHTTP(w, r)
}
