package services

import (
	"context"
	"errors"
	"testing"
)

type readinessStoreStub struct {
	err error
}

func (s readinessStoreStub) Ping(context.Context) error { return s.err }

func TestReadinessCheck(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		readiness := NewReadiness(readinessStoreStub{})
		if err := readiness.Check(context.Background()); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	})

	t.Run("persistence unavailable", func(t *testing.T) {
		databaseErr := errors.New("database unavailable")
		readiness := NewReadiness(readinessStoreStub{err: databaseErr})
		err := readiness.Check(context.Background())
		if !errors.Is(err, databaseErr) {
			t.Fatalf("Check() error = %v, want wrapped database error", err)
		}
	})
}
