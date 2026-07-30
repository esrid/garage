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
	TenantSettings(ctx context.Context, tenantID string) (Settings, error)
	UpdateTenantSettings(ctx context.Context, tenantID string, settings Settings) (Settings, error)
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

// Settings are what a workshop can change about itself from the desk: where a
// call is handed to a human, and how many voice minutes the month includes.
type Settings struct {
	Name                string
	Timezone            string
	TransferPhone       string
	MonthlyMinutesQuota int
}

// Settings reads the workshop's own configuration.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	tenantID, err := IDFromContext(ctx)
	if err != nil {
		return Settings{}, err
	}
	return s.store.TenantSettings(ctx, tenantID)
}

// UpdateSettings writes them back.
//
// The transfer number is normalised like every other phone in the system, and an
// empty one is allowed: a workshop that has nobody to transfer to should be able
// to say so rather than leave a wrong number in place.
func (s *Service) UpdateSettings(ctx context.Context, input Settings) (Settings, error) {
	tenantID, err := IDFromContext(ctx)
	if err != nil {
		return Settings{}, err
	}
	if strings.TrimSpace(input.TransferPhone) != "" {
		normalized, phoneErr := domain.NormalizePhone(input.TransferPhone)
		if phoneErr != nil {
			return Settings{}, &domain.ValidationError{
				Entity: "settings",
				Errors: map[string]string{"transfer_phone": "must be a reachable phone number"},
			}
		}
		input.TransferPhone = normalized
	} else {
		input.TransferPhone = ""
	}
	if input.MonthlyMinutesQuota < 1 || input.MonthlyMinutesQuota > 100_000 {
		return Settings{}, &domain.ValidationError{
			Entity: "settings",
			Errors: map[string]string{"monthly_minutes_quota": "must be between 1 and 100000"},
		}
	}
	return s.store.UpdateTenantSettings(ctx, tenantID, input)
}
