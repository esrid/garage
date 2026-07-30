package handlers

import (
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/esrid/garage/internal/web/views"
)

// Login serves GET /login. The F09 contract leaves this page to the frontend:
// the backend owns POST /auth/login, the cookie and the lockouts, and this
// handler owns nothing but the form.
//
// No dependency, no state, no session read: signed-in visitors are not redirected
// away, because knowing whether a cookie is valid is the middleware's job and
// duplicating that check here would be a second source of truth.
type Login struct{}

func NewLogin() *Login { return &Login{} }

func (h *Login) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", h.Page)
}

func (h *Login) Page(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	data := views.Login{
		Next:  safeNext(query.Get("next")),
		Error: query.Get("error"),
	}
	meta := views.SiteMeta{
		Title:       "Connexion — Atelier IA",
		Description: "Connexion à l'espace atelier.",
		Origin:      origin(r),
		Path:        "/login",
	}
	// A credentials form is never cached, and never indexed either: robots.txt
	// disallows /login for the crawlers that read it, this header covers the rest.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	templ.Handler(views.LoginPage(meta, data)).ServeHTTP(w, r)
}

// safeNext keeps only a local app path.
//
// The F09 middleware already encodes `next` from the request URI it saw, so this
// is defence in depth against a hand-written link: anything that is not a plain
// "/app..." path is dropped rather than sanitised, so no crafted value can turn
// the login form into an open redirect. A dropped value costs one extra click.
func safeNext(raw string) string {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return ""
	case !strings.HasPrefix(value, "/app"):
		return ""
	// "//host" is a protocol-relative URL, and a backslash is read as a slash by
	// some clients: both leave the site while looking local.
	case strings.HasPrefix(value, "//"), strings.Contains(value, "\\"):
		return ""
	case strings.ContainsAny(value, "\r\n"):
		return ""
	default:
		return value
	}
}
