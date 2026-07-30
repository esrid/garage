package tenant

import (
	"context"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/domain"
)

const DefaultTimezone = "America/Martinique"

type Tenant struct {
	ID        string
	Name      string
	Timezone  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateInput struct {
	Name     string
	Timezone string
}

type Store interface {
	CreateTenant(ctx context.Context, name, timezone string) (Tenant, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Tenant, error) {
	name := strings.TrimSpace(input.Name)
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = DefaultTimezone
	}

	validationErrors := make(map[string]string)
	if name == "" {
		validationErrors["name"] = "is required"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		validationErrors["timezone"] = "must be a valid IANA timezone"
	}
	if len(validationErrors) > 0 {
		return Tenant{}, &domain.ValidationError{Entity: "tenant", Errors: validationErrors}
	}

	return s.store.CreateTenant(ctx, name, timezone)
}
