package di

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/esrid/garage/internal/config"
)

func TestNewWiresReadinessSlice(t *testing.T) {
	cfg := testConfig(filepath.Join(t.TempDir(), "app.db"))
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
