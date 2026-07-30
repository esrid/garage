package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/adapters/handlers"
)

type readinessStub struct {
	err error
}

func (s readinessStub) Check(context.Context) error { return s.err }

func newHealthTestHandler(readiness readinessChecker) http.Handler {
	return New(readiness, handlers.NewDashboard(nil), handlers.NewAppointmentMutations(nil))
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
