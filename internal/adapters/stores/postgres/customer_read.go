package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
)

var _ customer.ReadStore = (*Store)(nil)

// SearchCustomers matches a name, a phone number or a plate: those are the three
// things a caller gives. An empty query lists the most recent customers, because
// a desk opening the page usually wants whoever just called.
//
// ILIKE with a leading wildcard cannot use a b-tree index. At the scale of one
// workshop that is a scan of a few thousand rows; a trigram index is the upgrade
// path when a tenant makes it hurt.
func (s *Store) SearchCustomers(ctx context.Context, tenantID, query string, limit int) ([]customer.Match, error) {
	const statement = `
		SELECT c.id::text, c.tenant_id::text, c.first_name, c.last_name, c.phone_e164,
			c.created_at, c.updated_at,
			COALESCE(array_agg(v.plate ORDER BY v.plate) FILTER (WHERE v.plate IS NOT NULL), '{}')
		FROM customers c
		LEFT JOIN vehicles v ON v.tenant_id = c.tenant_id AND v.customer_id = c.id
		WHERE c.tenant_id = $1::uuid
			AND ($2 = '' OR
				btrim(concat_ws(' ', c.first_name, c.last_name)) ILIKE '%' || $2 || '%' OR
				c.phone_e164 ILIKE '%' || $2 || '%' OR
				EXISTS (
					SELECT 1 FROM vehicles p
					WHERE p.tenant_id = c.tenant_id AND p.customer_id = c.id
						AND p.plate ILIKE '%' || $2 || '%'
				))
		GROUP BY c.id, c.tenant_id, c.first_name, c.last_name, c.phone_e164, c.created_at, c.updated_at
		ORDER BY c.created_at DESC, c.id
		LIMIT $3`

	rows, err := s.pool.Query(ctx, statement, tenantID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: search customers: %w", err)
	}
	defer rows.Close()

	matches := make([]customer.Match, 0, 16)
	for rows.Next() {
		var match customer.Match
		if err := rows.Scan(
			&match.ID, &match.TenantID, &match.FirstName, &match.LastName, &match.Phone,
			&match.CreatedAt, &match.UpdatedAt, &match.Plates,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan customer match: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: customer match rows: %w", err)
	}
	return matches, nil
}

// CustomerFile is one customer with their vehicles and their visits, newest
// first. Three queries rather than one join: a customer with four vehicles and
// twenty visits would otherwise come back as eighty rows to fold back together.
func (s *Store) CustomerFile(ctx context.Context, tenantID, customerID string) (customer.File, error) {
	var file customer.File
	const customerQuery = `
		SELECT c.id::text, c.tenant_id::text, c.first_name, c.last_name, c.phone_e164,
			c.created_at, c.updated_at, t.timezone
		FROM customers c
		JOIN tenants t ON t.id = c.tenant_id
		WHERE c.tenant_id = $1::uuid AND c.id = $2::uuid`

	if err := s.pool.QueryRow(ctx, customerQuery, tenantID, customerID).Scan(
		&file.Customer.ID, &file.Customer.TenantID, &file.Customer.FirstName,
		&file.Customer.LastName, &file.Customer.Phone,
		&file.Customer.CreatedAt, &file.Customer.UpdatedAt, &file.Timezone,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return customer.File{}, &domain.NotFoundError{Entity: "customer"}
		}
		return customer.File{}, fmt.Errorf("postgres: customer file: %w", err)
	}

	vehicleRows, err := s.pool.Query(ctx, `
		SELECT id::text, COALESCE(plate, ''), make, model
		FROM vehicles
		WHERE tenant_id = $1::uuid AND customer_id = $2::uuid
		ORDER BY created_at, id`, tenantID, customerID)
	if err != nil {
		return customer.File{}, fmt.Errorf("postgres: customer vehicles: %w", err)
	}
	for vehicleRows.Next() {
		var value customer.Vehicle
		if err := vehicleRows.Scan(&value.ID, &value.Plate, &value.Make, &value.Model); err != nil {
			vehicleRows.Close()
			return customer.File{}, fmt.Errorf("postgres: scan customer vehicle: %w", err)
		}
		file.Vehicles = append(file.Vehicles, value)
	}
	vehicleRows.Close()
	if err := vehicleRows.Err(); err != nil {
		return customer.File{}, fmt.Errorf("postgres: customer vehicle rows: %w", err)
	}

	visitRows, err := s.pool.Query(ctx, `
		SELECT a.id::text, a.starts_at, a.service_label, a.status, COALESCE(v.plate, '')
		FROM appointments a
		LEFT JOIN vehicles v ON v.tenant_id = a.tenant_id AND v.id = a.vehicle_id
		WHERE a.tenant_id = $1::uuid AND a.customer_id = $2::uuid
		ORDER BY a.starts_at DESC, a.id
		LIMIT 50`, tenantID, customerID)
	if err != nil {
		return customer.File{}, fmt.Errorf("postgres: customer visits: %w", err)
	}
	defer visitRows.Close()
	for visitRows.Next() {
		var value customer.Visit
		if err := visitRows.Scan(&value.ID, &value.Start, &value.ServiceLabel, &value.Status, &value.Plate); err != nil {
			return customer.File{}, fmt.Errorf("postgres: scan customer visit: %w", err)
		}
		file.Visits = append(file.Visits, value)
	}
	if err := visitRows.Err(); err != nil {
		return customer.File{}, fmt.Errorf("postgres: customer visit rows: %w", err)
	}
	return file, nil
}
