package di

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/config"
	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/tenant"
)

func TestNewWiresReadinessAndApplicationRoutes(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}
	cfg := testConfig(dsn)
	app, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if app.server.MaxHeaderBytes != cfg.MaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", app.server.MaxHeaderBytes, cfg.MaxHeaderBytes)
	}

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, want %d", response.Code, http.StatusOK)
	}

	request = httptest.NewRequest(http.MethodGet, "/app", nil)
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("dashboard status=%d body=%q, want session protection", response.Code, response.Body.String())
	}

	database, ok := app.database.(interface {
		tenant.Store
		coreauth.Store
	})
	if !ok {
		t.Fatal("wired database does not implement auth stores")
	}
	tenantValue, err := tenant.NewService(database).Create(context.Background(), tenant.CreateInput{Name: "Garage DI auth"})
	if err != nil {
		t.Fatalf("create auth tenant: %v", err)
	}
	email := fmt.Sprintf("di-auth-%d@example.com", time.Now().UnixNano())
	_, err = coreauth.NewService(database).Provision(tenant.WithID(context.Background(), tenantValue.ID), coreauth.ProvisionInput{
		Email: email, Password: "correct horse battery staple", Role: coreauth.RoleOwner,
	})
	if err != nil {
		t.Fatalf("provision auth staff: %v", err)
	}
	loginForm := url.Values{"email": {email}, "password": {"correct horse battery staple"}}
	request = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginForm.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/app" {
		t.Fatalf("login status=%d location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	loginCookies := response.Result().Cookies()
	if len(loginCookies) != 1 {
		t.Fatalf("login cookies = %#v", loginCookies)
	}
	request = httptest.NewRequest(http.MethodGet, "/app", nil)
	request.AddCookie(loginCookies[0])
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "momentanément indisponibles") {
		t.Fatalf("authenticated dashboard status=%d body=%q", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/app/calls", nil)
	request.AddCookie(loginCookies[0])
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Appels du jour") {
		t.Fatalf("call history status=%d body=%q", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/app/calls/not-a-uuid", nil)
	request.AddCookie(loginCookies[0])
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "Appel introuvable") {
		t.Fatalf("unknown call status=%d body=%q", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(loginCookies[0])
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("logout status=%d body=%q", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/app", nil)
	request.AddCookie(loginCookies[0])
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked dashboard status=%d body=%q", response.Code, response.Body.String())
	}

	appointmentID := "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea3"
	routes := []struct {
		path string
		form url.Values
	}{
		{
			"/app/appointments",
			url.Values{
				"customer_id":      {"019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1"},
				"service_label":    {"Révision"},
				"start_at":         {"2030-01-02T08:00:00-04:00"},
				"duration_minutes": {"60"},
				"idempotency_key":  {"di-book-route"},
			},
		},
		{
			"/app/appointments/" + appointmentID + "/reschedule",
			url.Values{
				"start_at":         {"2030-01-02T09:00:00-04:00"},
				"duration_minutes": {"60"},
				"idempotency_key":  {"di-move-route"},
			},
		},
		{
			"/app/appointments/" + appointmentID + "/cancel",
			url.Values{"idempotency_key": {"di-cancel-route"}},
		},
	}
	for _, route := range routes {
		request = httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response = httptest.NewRecorder()
		app.server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("POST %s status = %d, want %d", route.path, response.Code, http.StatusUnauthorized)
		}
	}

	request = httptest.NewRequest(http.MethodPost, "/voice/tools/customer-lookup", strings.NewReader(`{"phone":"+596696123456"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("voice lookup status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	voiceRoutes := []struct {
		path string
		body string
	}{
		{"/voice/tools/appointment-availability", `{"day":"2030-01-02T12:00:00-04:00","duration_minutes":60}`},
		{"/voice/tools/appointment-book", `{"conversation_id":"conv","customer_id":"019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1","service_label":"Révision","start_at":"2030-01-02T08:00:00-04:00","duration_minutes":60}`},
		{"/voice/tools/follow-up-request", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":"Rappeler."}`},
	}
	for _, route := range voiceRoutes {
		request = httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.body))
		request.Header.Set("Content-Type", "application/json")
		response = httptest.NewRecorder()
		app.server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("POST %s status = %d, want %d", route.path, response.Code, http.StatusUnauthorized)
		}
	}

	request = httptest.NewRequest(http.MethodPost, "/webhooks/elevenlabs/post-call", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("disabled post-call webhook status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestNewRejectsInvalidVoiceCredentialsBeforeOpeningDatabase(t *testing.T) {
	cfg := testConfig("not-a-database-dsn")
	cfg.VoiceToolTokens = "invalid"
	_, err := New(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "voice credentials") {
		t.Fatalf("New() error = %v, want voice credentials error", err)
	}
}

func testConfig(dsn string) config.Config {
	return config.Config{
		HTTPAddr:          "127.0.0.1:8080",
		DatabaseDSN:       dsn,
		MaxHeaderBytes:    64 << 10,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   time.Second,
	}
}
