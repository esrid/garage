package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("DATABASE_DSN", "postgres://garage@localhost:5432/garage?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.DatabaseDSN != "postgres://garage@localhost:5432/garage?sslmode=disable" {
		t.Fatalf("DatabaseDSN = %q", cfg.DatabaseDSN)
	}
	if cfg.MaxHeaderBytes != 64<<10 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", cfg.MaxHeaderBytes, 64<<10)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 10s", cfg.ShutdownTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("DATABASE_DSN", "postgres://garage@db:5432/custom")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "32768")
	t.Setenv("HTTP_READ_TIMEOUT", "3s")
	t.Setenv("VOICE_TOOL_TOKENS", "tenant:secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" || cfg.DatabaseDSN != "postgres://garage@db:5432/custom" || cfg.VoiceToolTokens != "tenant:secret" || cfg.MaxHeaderBytes != 32768 || cfg.ReadTimeout != 3*time.Second {
		t.Fatalf("Load() = %#v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Run("missing database DSN", func(t *testing.T) {
		clearConfigEnvironment(t)
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "DATABASE_DSN") {
			t.Fatalf("Load() error = %v, want DATABASE_DSN error", err)
		}
	})

	t.Run("address", func(t *testing.T) {
		clearConfigEnvironment(t)
		t.Setenv("DATABASE_DSN", "postgres://garage@localhost:5432/garage")
		t.Setenv("HTTP_ADDR", "missing-port")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "HTTP_ADDR") {
			t.Fatalf("Load() error = %v, want HTTP_ADDR error", err)
		}
	})

	t.Run("duration", func(t *testing.T) {
		clearConfigEnvironment(t)
		t.Setenv("DATABASE_DSN", "postgres://garage@localhost:5432/garage")
		t.Setenv("SHUTDOWN_TIMEOUT", "later")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "SHUTDOWN_TIMEOUT") {
			t.Fatalf("Load() error = %v, want SHUTDOWN_TIMEOUT error", err)
		}
	})

	t.Run("header bytes", func(t *testing.T) {
		clearConfigEnvironment(t)
		t.Setenv("DATABASE_DSN", "postgres://garage@localhost:5432/garage")
		t.Setenv("HTTP_MAX_HEADER_BYTES", "many")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "HTTP_MAX_HEADER_BYTES") {
			t.Fatalf("Load() error = %v, want HTTP_MAX_HEADER_BYTES error", err)
		}
	})
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"HTTP_ADDR",
		"DATABASE_DSN",
		"DSN",
		"VOICE_TOOL_TOKENS",
		"HTTP_MAX_HEADER_BYTES",
		"HTTP_READ_HEADER_TIMEOUT",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"SHUTDOWN_TIMEOUT",
	} {
		t.Setenv(name, "")
	}
}
