package postgres

import (
	"context"
	"fmt"

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
