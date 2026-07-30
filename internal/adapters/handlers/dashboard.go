// Package handlers holds the HTTP handlers for the application pages.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/esrid/garage/internal/web/views"
)

// TodayProvider supplies the day view. Frozen in
// docs/contracts/F04-dashboard-today.md.
//
// tenant_id is deliberately absent: it travels in ctx, put there by the tenant
// middleware, so a frontend caller cannot pass one and break the invariant
// (PRD 7.1).
type TodayProvider interface {
	Today(ctx context.Context, day time.Time) (views.Today, error)
}

// Dashboard renders the day view.
type Dashboard struct {
	provider TodayProvider
	// now is injected so the rendered day is a function of input, not of the
	// wall clock, which keeps the handler testable.
	now func() time.Time
}

func NewDashboard(provider TodayProvider) *Dashboard {
	return &Dashboard{provider: provider, now: time.Now}
}

// Page serves GET /app.
func (d *Dashboard) Page(w http.ResponseWriter, r *http.Request) {
	data, degraded := d.today(r)
	d.render(w, r, views.DashboardPage(data, degraded))
}

// Fragment serves GET /app/today: the panels alone, for an htmx refresh.
func (d *Dashboard) Fragment(w http.ResponseWriter, r *http.Request) {
	data, degraded := d.today(r)
	d.render(w, r, views.TodayPanels(data, degraded))
}

// today never returns an error: a failing provider degrades the page instead of
// blanking it. The operator still sees an empty dashboard rather than a 500,
// and the reason is logged.
func (d *Dashboard) today(r *http.Request) (views.Today, bool) {
	day := d.now()
	data, err := d.provider.Today(r.Context(), day)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboard: today unavailable", "err", err)
		return views.Today{Day: day}, true
	}
	if data.Day.IsZero() {
		data.Day = day
	}
	return data, false
}

func (d *Dashboard) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	// The day view is live operational data: a cached copy shown after a back
	// navigation would be misleading.
	w.Header().Set("Cache-Control", "no-store")
	// templ.Handler buffers by default, so a mid-render error cannot emit half
	// a page followed by a 200.
	templ.Handler(component).ServeHTTP(w, r)
}
