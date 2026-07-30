package customerfiles

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/web/page"
)

// Handler serves the customer files: who called, what they drive, when they came.
//
// The domain has existed since F01 and had no screen at all, so a garage could
// not look anyone up. Read-only for now: the assistant writes these records
// during a call (F19), and a desk edit form is a separate decision.
type Handler struct {
	reader customer.FileReader
	now    func() time.Time
}

func NewHandler(reader customer.FileReader) *Handler {
	return &Handler{reader: reader, now: time.Now}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/customers", h.Search)
	mux.HandleFunc("GET /app/customers/{id}", h.File)
}

// Search serves GET /app/customers?q=. An empty query lists the most recent.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	matches, err := h.reader.Search(r.Context(), query)
	if err != nil {
		view, status := h.unavailable(r.Context(), err)
		view.Query = query
		page.Render(w, r, status, SearchPage(view))
		return
	}
	page.Render(w, r, http.StatusOK, SearchPage(SearchView{Query: query, Matches: matches}))
}

// File serves GET /app/customers/{id}.
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	file, err := h.reader.File(r.Context(), r.PathValue("id"))
	if err != nil {
		var notFound *domain.NotFoundError
		if errors.As(err, &notFound) {
			// One page for an unknown id and for another workshop's customer: an id
			// must not become a way to ask whether someone exists elsewhere.
			page.Render(w, r, http.StatusNotFound, ProblemPage(
				"Fiche introuvable",
				"Cette fiche n'existe pas, ou n'appartient pas à cet atelier.",
			))
			return
		}
		slog.ErrorContext(r.Context(), "customers: file unavailable", "err", err)
		page.Render(w, r, http.StatusOK, ProblemPage(
			"Fiche indisponible",
			"Cette fiche n'a pas pu être lue. Réessayez dans un instant.",
		))
		return
	}
	page.Render(w, r, http.StatusOK, FilePage(file))
}

func (h *Handler) unavailable(ctx context.Context, err error) (SearchView, int) {
	var unauthorized *domain.UnauthorizedError
	if errors.As(err, &unauthorized) {
		return SearchView{Degraded: true, Notice: "Connexion requise."}, http.StatusUnauthorized
	}
	slog.ErrorContext(ctx, "customers: search unavailable", "err", err)
	return SearchView{
		Degraded: true,
		Notice:   "La recherche est momentanément indisponible. Réessayez dans un instant.",
	}, http.StatusOK
}
