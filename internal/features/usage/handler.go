package usage

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/web/page"
)

// Handler shows a workshop where it stands against its monthly voice minutes.
//
// The PRD's economic rule is that no plan is unlimited and the workshop is warned
// at 70, 85 and 100 % of its quota. The pricing page has been promising that
// since the first day; this is the screen that makes it true.
type Handler struct {
	reader conversation.UsageReader
	// now is injected so the rendered month is a function of input, not of the
	// wall clock.
	now func() time.Time
}

func NewHandler(reader conversation.UsageReader) *Handler {
	return &Handler{reader: reader, now: time.Now}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/usage", h.Page)
}

// Page serves GET /app/usage?month=YYYY-MM.
func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	data, status := h.load(r)
	page.Render(w, r, status, UsagePage(data))
}

func (h *Handler) load(r *http.Request) (View, int) {
	ctx := r.Context()
	current, err := h.reader.Usage(ctx, h.now())
	if err != nil {
		return h.unavailable(ctx, err)
	}

	view := View{Usage: current}
	raw := strings.TrimSpace(r.URL.Query().Get("month"))
	if raw == "" {
		return view, http.StatusOK
	}
	// A month is a civil date like a day: it only means something in the
	// workshop's timezone, which the read just reported.
	requested, parseErr := time.ParseInLocation("2006-01", raw, current.Month.Location())
	if parseErr != nil {
		view.Notices = append(view.Notices, "Mois illisible : voici le mois en cours.")
		return view, http.StatusOK
	}
	asked, err := h.reader.Usage(ctx, requested.AddDate(0, 0, 14))
	if err != nil {
		return h.unavailable(ctx, err)
	}
	return View{Usage: asked}, http.StatusOK
}

func (h *Handler) unavailable(ctx context.Context, err error) (View, int) {
	var unauthorized *domain.UnauthorizedError
	if errors.As(err, &unauthorized) {
		return View{Degraded: true, Notices: []string{"Connexion requise."}}, http.StatusUnauthorized
	}
	slog.ErrorContext(ctx, "usage: consumption unavailable", "err", err)
	return View{
		Degraded: true,
		Notices:  []string{"La consommation est momentanément indisponible. Réessayez dans un instant."},
	}, http.StatusOK
}
