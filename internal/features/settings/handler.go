package settings

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
	"github.com/esrid/garage/internal/web/page"
)

const maxSettingsFormBytes = 8 << 10

// Handler lets a workshop set the two things it owns and could not reach: the
// number a call is handed to a human on, and the monthly voice minutes it bought.
//
// Both existed only in a database column or on the pricing page. A value nobody
// can change is the same trap the opening hours were in.
type Handler struct {
	tenants *tenant.Service
}

func NewHandler(tenants *tenant.Service) *Handler {
	return &Handler{tenants: tenants}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/settings", h.Page)
	mux.HandleFunc("POST /app/settings", h.Save)
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	current, err := h.tenants.Settings(r.Context())
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	page.Render(w, r, http.StatusOK, SettingsPage(View{
		Settings: current,
		Saved:    r.URL.Query().Get("saved") == "1",
		Notice:   noticeFor(r.URL.Query().Get("error")),
	}))
}

// Save serves POST /app/settings and answers with a redirect, so a refresh after
// saving does not offer to submit the form again.
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		h.redirect(w, r, "invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsFormBytes)
	if err := r.ParseForm(); err != nil {
		h.redirect(w, r, "invalid")
		return
	}
	quota, convErr := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("monthly_minutes_quota")))
	if convErr != nil {
		h.redirect(w, r, "invalid")
		return
	}

	if _, err := h.tenants.UpdateSettings(r.Context(), tenant.Settings{
		TransferPhone:       r.PostForm.Get("transfer_phone"),
		MonthlyMinutesQuota: quota,
	}); err != nil {
		var validation *domain.ValidationError
		var unauthorized *domain.UnauthorizedError
		switch {
		case errors.As(err, &validation):
			h.redirect(w, r, "invalid")
		case errors.As(err, &unauthorized):
			http.Error(w, "authentication required", http.StatusUnauthorized)
		default:
			h.redirect(w, r, "unavailable")
		}
		return
	}
	http.Redirect(w, r, "/app/settings?saved=1", http.StatusSeeOther)
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/app/settings?"+url.Values{"error": {code}}.Encode(), http.StatusSeeOther)
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, err error) {
	var unauthorized *domain.UnauthorizedError
	if errors.As(err, &unauthorized) {
		page.Render(w, r, http.StatusUnauthorized, SettingsPage(View{
			Degraded: true, Notice: "Connexion requise.",
		}))
		return
	}
	page.Render(w, r, http.StatusOK, SettingsPage(View{
		Degraded: true,
		Notice:   "Les réglages sont momentanément indisponibles. Réessayez dans un instant.",
	}))
}

// noticeFor turns the closed error code carried by the redirect into a sentence.
// An unknown code reads as the generic failure rather than reaching the page.
func noticeFor(code string) string {
	switch code {
	case "":
		return ""
	case "invalid":
		return "Réglages refusés : vérifiez le numéro (format international) et le quota."
	default:
		return "Les réglages n'ont pas pu être enregistrés. Réessayez dans un instant."
	}
}
