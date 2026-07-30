package services

import (
	"context"
	"fmt"
)

// ReadinessStore is the persistence capability this use-case needs. Declared
// here, next to the only code that consumes it, like every other port in the
// core: a database adapter satisfies it without its driver ever reaching this
// package.
//
// It used to live alone in an internal/core/ports package, a convention the
// fourteen other ports did not follow. One rule now: a port is declared by
// whoever needs it.
type ReadinessStore interface {
	Ping(context.Context) error
}

type Readiness struct {
	store ReadinessStore
}

func NewReadiness(store ReadinessStore) *Readiness {
	return &Readiness{store: store}
}

func (r *Readiness) Check(ctx context.Context) error {
	if err := r.store.Ping(ctx); err != nil {
		return fmt.Errorf("readiness: persistence: %w", err)
	}
	return nil
}
