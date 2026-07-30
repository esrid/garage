package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func loginPage(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewLogin().Register(mux)
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = "atelier.example"
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestLoginPageRendersAUsableForm(t *testing.T) {
	response := loginPage(t, "/login")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()

	for _, want := range []string{
		`method="post" action="/auth/login"`, // the frozen F09 route
		`name="email"`,
		`name="password"`,
		`type="password"`,
		`autocomplete="username"`,         // the password manager must offer the saved
		`autocomplete="current-password"`, // credential, not propose a new one
		`for="login-email"`,
		`for="login-password"`,
		"required",
		"<h1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("login page is missing %s", want)
		}
	}
	// A credentials page must not be cached or indexed.
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Errorf("X-Robots-Tag = %q, want noindex", got)
	}
}

// The page must never suggest whether the account exists: F09 goes to real
// lengths to keep failures indistinguishable, and a chattier message here would
// undo it.
func TestLoginErrorsStayVague(t *testing.T) {
	cases := map[string]string{
		"invalid":      "Email ou mot de passe incorrect.",
		"rate_limited": "Trop de tentatives",
		"unavailable":  "momentanément indisponible",
		"expired":      "session a expiré",
		// An unknown code must not reach the page as raw text.
		"wat":                       "Email ou mot de passe incorrect.",
		"<script>alert(1)</script>": "Email ou mot de passe incorrect.",
	}

	for code, want := range cases {
		body := loginPage(t, "/login?error="+code).Body.String()
		if !strings.Contains(body, want) {
			t.Errorf("error=%q does not show %q", code, want)
		}
		// Claims about the account itself. Not a bare "n'existe pas": the page says
		// self-service reset does not exist yet, which is about the product.
		for _, forbidden := range []string{
			"compte inconnu", "email inconnu", "utilisateur inconnu",
			"ce compte n'existe pas", "compte verrouillé", "compte désactivé", "<script>",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("error=%q leaks %q", code, forbidden)
			}
		}
	}

	// No error, no message.
	if body := loginPage(t, "/login").Body.String(); strings.Contains(body, `role="alert"`) {
		t.Error("the page shows an alert with nothing to report")
	}
}

// The return path comes from the F09 middleware, which builds it from the request
// URI it saw. Anything else is dropped rather than sanitised: a login form is
// exactly where an open redirect is worth the most to an attacker.
func TestLoginKeepsOnlyLocalReturnPaths(t *testing.T) {
	kept := []string{"/app", "/app/planning", "/app/planning?day=2026-07-31"}
	for _, next := range kept {
		body := loginPage(t, "/login?next="+next).Body.String()
		if !strings.Contains(body, `name="next"`) {
			t.Errorf("next=%q was dropped", next)
		}
	}

	dropped := []string{
		"https://evil.example/app",
		"//evil.example",
		"/appearance-is-not-app", // prefix match must not be enough on its own
		"\\/evil.example",
		"/",
		"/tarifs",
		"javascript:alert(1)",
	}
	for _, next := range dropped {
		body := loginPage(t, "/login?next="+next).Body.String()
		if strings.Contains(body, "evil.example") || strings.Contains(body, "javascript:") {
			t.Errorf("next=%q reached the page", next)
		}
	}
}

// Signing out has to be a POST: F09 revokes the server session there, and a GET
// logout is firable by any third-party image tag.
func TestAppShellSignsOutWithAForm(t *testing.T) {
	body := get(t, newTestDashboard(&stubProvider{data: dashboardPreviewData()}).Page, "/app").Body.String()

	if !strings.Contains(body, `method="post" action="/auth/logout"`) {
		t.Error("the app shell has no logout form")
	}
	if strings.Contains(body, `href="/auth/logout"`) {
		t.Error("logout must not be a link")
	}
}

func TestRobotsKeepsCrawlersOffTheLoginForm(t *testing.T) {
	if body := fetch(t, "/robots.txt").Body.String(); !strings.Contains(body, "Disallow: /login") {
		t.Error("robots.txt does not disallow /login")
	}
}
