package services

import (
	"context"
	"fmt"

	"github.com/esrid/garage/internal/core/ports"
)

type Readiness struct {
	store ports.ReadinessStore
}

func NewReadiness(store ports.ReadinessStore) *Readiness {
	return &Readiness{store: store}
}

func (r *Readiness) Check(ctx context.Context) error {
	if err := r.store.Ping(ctx); err != nil {
		return fmt.Errorf("readiness: persistence: %w", err)
	}
	return nil
}
