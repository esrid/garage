package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	return New(
		readiness,
		handlers.NewDashboard(nil),
		handlers.NewPlanning(nil),
		handlers.NewAppointmentMutations(nil),
		voice.NewCustomerLookup(nil, nil),
		voice.NewAppointmentTools(nil, nil),
		voice.NewFollowUpTool(nil, nil),
		handlers.NewAuthentication(nil),
		sessionVerifierStub{},
	)
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

func TestApplicationRoutesRequireSessionAndRejectCrossOriginPosts(t *testing.T) {
	handler := newHealthTestHandler(readinessStub{})
	request := httptest.NewRequest(http.MethodGet, "/app", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("GET /app status = %d, want 401", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin logout status = %d, want 403", response.Code)
	}
}
