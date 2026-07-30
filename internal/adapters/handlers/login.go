package handlers

import (
	"net/http"
	"net/url"
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
// net/url does the parsing: a login form is exactly where an open redirect is
// worth the most, and deciding what "local" means by looking for prefixes is how
// "//evil.example" or a backslash slips through. A URL that carries a scheme, a
// host, or a user is not a path on this site, whatever it looks like.
//
// The F09 middleware already builds `next` from the request URI it saw, so this
// is defence in depth against a hand-written link.
func safeNext(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || strings.ContainsAny(value, "\r\n\\") {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" {
		return ""
	}
	// Only the staff area is worth returning to, and only as an absolute path.
	if !strings.HasPrefix(parsed.Path, "/app") {
		return ""
	}
	return value
}
