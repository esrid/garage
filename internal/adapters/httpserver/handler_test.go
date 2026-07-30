package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/adapters/handlers"
	"github.com/esrid/garage/internal/adapters/voice"
	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type readinessStub struct {
	err error
}

func (s readinessStub) Check(context.Context) error { return s.err }

type sessionVerifierStub struct {
	identity coreauth.Identity
	err      error
}

func (s sessionVerifierStub) Resume(context.Context, string) (coreauth.Identity, error) {
	return s.identity, s.err
}

func newHealthTestHandler(readiness readinessChecker) http.Handler {
	return New(Deps{
		Readiness:        readiness,
		Sessions:         sessionVerifierStub{},
		Authentication:   handlers.NewAuthentication(nil),
		Dashboard:        handlers.NewDashboard(nil),
		Calls:            handlers.NewCalls(nil),
		Planning:         handlers.NewPlanning(nil),
		Appointments:     handlers.NewAppointmentMutations(nil),
		CustomerLookup:   voice.NewCustomerLookup(nil, nil),
		AppointmentTools: voice.NewAppointmentTools(nil, nil),
		FollowUpTool:     voice.NewFollowUpTool(nil, nil),
		PostCallWebhook:  mustDisabledPostCallWebhook(),
	})
}

func mustDisabledPostCallWebhook() *voice.PostCallWebhook {
	handler, err := voice.NewPostCallWebhook(nil, "", "")
	if err != nil {
		panic(err)
	}
	return handler
}

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		readiness  error
		wantStatus int
		wantBody   string
	}{
		{"health", "/healthz", errors.New("ignored"), http.StatusOK, `"status":"ok"`},
		{"ready", "/readyz", nil, http.StatusOK, `"status":"ready"`},
		{"not ready", "/readyz", errors.New("down"), http.StatusServiceUnavailable, `"status":"unavailable"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			newHealthTestHandler(readinessStub{err: test.readiness}).ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.wantBody)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("X-Request-ID header is empty")
			}
			if response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("security headers were not applied")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("health response is cacheable")
			}
		})
	}
}

func TestHealthEndpointRejectsOtherMethods(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()
	newHealthTestHandler(readinessStub{}).ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestRecoverPanic(t *testing.T) {
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	handler := requestID(recoverPanic(panicking))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestRequireStaffSessionEstablishesIdentityAndTenantContext(t *testing.T) {
	want := coreauth.Identity{StaffID: "staff-1", TenantID: "tenant-1", Email: "garage@example.com", Role: coreauth.RoleOwner}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		identity, err := coreauth.IdentityFromContext(r.Context())
		if err != nil || identity != want {
			t.Errorf("identity = %#v, %v", identity, err)
		}
		tenantID, err := tenant.IDFromContext(r.Context())
		if err != nil || tenantID != want.TenantID {
			t.Errorf("tenant = %q, %v", tenantID, err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := requireStaffSession(sessionVerifierStub{identity: want}, next)
	request := httptest.NewRequest(http.MethodGet, "/app", nil)
	request.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%v body=%q", response.Code, called, response.Body.String())
	}
}

func TestRequireStaffSessionRejectsMissingInvalidAndUnavailableSessions(t *testing.T) {
	tests := []struct {
		name       string
		withCookie bool
		err        error
		want       int
	}{
		{"missing", false, nil, http.StatusUnauthorized},
		{"invalid", true, &domain.UnauthorizedError{}, http.StatusUnauthorized},
		{"unavailable", true, errors.New("database down"), http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := requireStaffSession(sessionVerifierStub{err: test.err}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			request := httptest.NewRequest(http.MethodGet, "/app", nil)
			if test.withCookie {
				request.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "session-token"})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want || called {
				t.Fatalf("status=%d want=%d called=%v", response.Code, test.want, called)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("authentication response is cacheable")
			}
		})
	}
}

func TestRequireStaffSessionSelectsBrowserReauthenticationResponse(t *testing.T) {
	tests := []struct {
		name         string
		withCookie   bool
		verifierErr  error
		headers      map[string]string
		wantStatus   int
		wantLocation string
		wantHX       string
	}{
		{
			name: "navigation by fetch mode", headers: map[string]string{"Sec-Fetch-Mode": "navigate"},
			wantStatus: http.StatusSeeOther, wantLocation: "/login?next=%2Fapp%2Fplanning%3Fday%3D2030-01-02",
		},
		{
			name: "navigation by accept", withCookie: true, verifierErr: &domain.UnauthorizedError{},
			headers:    map[string]string{"Accept": "text/html,application/xhtml+xml"},
			wantStatus: http.StatusSeeOther, wantLocation: "/login?next=%2Fapp%2Fplanning%3Fday%3D2030-01-02",
		},
		{
			name: "htmx takes precedence", withCookie: true, verifierErr: &domain.UnauthorizedError{},
			headers:    map[string]string{"HX-Request": "true", "Accept": "text/html"},
			wantStatus: http.StatusUnauthorized, wantHX: "/login",
		},
		{
			name: "store failure is not session expiry", withCookie: true, verifierErr: errors.New("database down"),
			headers: map[string]string{"Sec-Fetch-Mode": "navigate"}, wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := requireStaffSession(sessionVerifierStub{err: test.verifierErr}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			request := httptest.NewRequest(http.MethodGet, "/app/planning?day=2030-01-02", nil)
			if test.withCookie {
				request.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "session-token"})
			}
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || called {
				t.Fatalf("status=%d want=%d called=%v body=%q", response.Code, test.wantStatus, called, response.Body.String())
			}
			if got := response.Header().Get("Location"); got != test.wantLocation {
				t.Fatalf("Location = %q, want %q", got, test.wantLocation)
			}
			if got := response.Header().Get("HX-Redirect"); got != test.wantHX {
				t.Fatalf("HX-Redirect = %q, want %q", got, test.wantHX)
			}
		})
	}
}

func TestSessionNavigationNextCannotBecomeOpenRedirect(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/app?next=https://attacker.example", nil)
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	requireStaffSession(sessionVerifierStub{}, http.NotFoundHandler()).ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", response.Code)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if location.IsAbs() || location.Path != "/login" || !strings.HasPrefix(location.Query().Get("next"), "/app?") {
		t.Fatalf("unsafe login redirect = %q", location.String())
	}
}

func TestApplicationRoutesRequireSessionAndRejectCrossOriginPosts(t *testing.T) {
	handler := newHealthTestHandler(readinessStub{})
	for _, path := range []string{"/app", "/app/calls", "/app/calls/call-id"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want 401", path, response.Code)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin logout status = %d, want 403", response.Code)
	}
}

// Only a GET can be replayed by following a link after signing in. A POST URI in
// next sends the garage to a route with no GET handler once they are back.
func TestSessionNextIsOnlyCarriedForReplayableRequests(t *testing.T) {
	tests := map[string]struct {
		method       string
		target       string
		wantLocation string
	}{
		"get keeps its path":   {http.MethodGet, "/app/planning?day=2030-01-02", "/login?next=%2Fapp%2Fplanning%3Fday%3D2030-01-02"},
		"post carries no next": {http.MethodPost, "/app/appointments/019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1/cancel", "/login"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			response := httptest.NewRecorder()

			requireStaffSession(sessionVerifierStub{err: &domain.UnauthorizedError{}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("the protected handler must not run")
			})).ServeHTTP(response, request)

			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", response.Code)
			}
			if got := response.Header().Get("Location"); got != test.wantLocation {
				t.Errorf("Location = %q, want %q", got, test.wantLocation)
			}
		})
	}
}
