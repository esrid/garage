package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

var _ tenant.Store = (*Store)(nil)

func (s *Store) CreateTenant(ctx context.Context, name, timezone string) (tenant.Tenant, error) {
	const query = `
		INSERT INTO tenants (name, timezone)
		VALUES ($1, $2)
		RETURNING id::text, name, timezone, created_at, updated_at`

	var created tenant.Tenant
	err := s.pool.QueryRow(ctx, query, name, timezone).Scan(
		&created.ID,
		&created.Name,
		&created.Timezone,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("postgres: create tenant: %w", err)
	}
	return created, nil
}

// TenantSettings reads what a workshop can change about itself.
func (s *Store) TenantSettings(ctx context.Context, tenantID string) (tenant.Settings, error) {
	const query = `
		SELECT name, timezone, COALESCE(transfer_phone_e164, ''), monthly_minutes_quota
		FROM tenants WHERE id = $1::uuid`

	var settings tenant.Settings
	if err := s.pool.QueryRow(ctx, query, tenantID).Scan(
		&settings.Name, &settings.Timezone, &settings.TransferPhone, &settings.MonthlyMinutesQuota,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenant.Settings{}, &domain.NotFoundError{Entity: "tenant"}
		}
		return tenant.Settings{}, fmt.Errorf("postgres: tenant settings: %w", err)
	}
	return settings, nil
}

// UpdateTenantSettings writes them back. The name and the timezone are not
// touched here: renaming a workshop or moving it to another timezone would
// reinterpret every stored instant, and that is an onboarding decision.
func (s *Store) UpdateTenantSettings(ctx context.Context, tenantID string, settings tenant.Settings) (tenant.Settings, error) {
	const query = `
		UPDATE tenants
		SET transfer_phone_e164 = NULLIF($2, ''), monthly_minutes_quota = $3, updated_at = now()
		WHERE id = $1::uuid
		RETURNING name, timezone, COALESCE(transfer_phone_e164, ''), monthly_minutes_quota`

	var updated tenant.Settings
	if err := s.pool.QueryRow(ctx, query, tenantID, settings.TransferPhone, settings.MonthlyMinutesQuota).Scan(
		&updated.Name, &updated.Timezone, &updated.TransferPhone, &updated.MonthlyMinutesQuota,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenant.Settings{}, &domain.NotFoundError{Entity: "tenant"}
		}
		return tenant.Settings{}, fmt.Errorf("postgres: update tenant settings: %w", err)
	}
	return updated, nil
}
