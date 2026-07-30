package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/vehicle"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ vehicle.Store = (*Store)(nil)

const vehicleColumns = `
	id::text, tenant_id::text, customer_id::text, plate,
	COALESCE(plate_normalized, ''), make, model, created_at, updated_at`

func (s *Store) CreateVehicle(ctx context.Context, tenantID string, input vehicle.CreateInput, normalizedPlate string) (vehicle.Vehicle, error) {
	query := `
		INSERT INTO vehicles (tenant_id, customer_id, plate, plate_normalized, make, model)
		VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, ''), $5, $6)
		RETURNING ` + vehicleColumns

	created, err := scanVehicle(s.pool.QueryRow(
		ctx,
		query,
		tenantID,
		input.CustomerID,
		input.Plate,
		normalizedPlate,
		input.Make,
		input.Model,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch {
			case pgErr.Code == "23505" && pgErr.ConstraintName == "vehicles_tenant_plate_key":
				return vehicle.Vehicle{}, &domain.AlreadyExistsError{Entity: "vehicle", Field: "plate", Value: input.Plate}
			case pgErr.Code == "23503" && pgErr.ConstraintName == "vehicles_tenant_customer_fkey":
				return vehicle.Vehicle{}, &domain.NotFoundError{Entity: "customer", ID: input.CustomerID}
			}
		}
		return vehicle.Vehicle{}, fmt.Errorf("postgres: create vehicle: %w", err)
	}
	return created, nil
}

func (s *Store) FindVehicleByPlate(ctx context.Context, tenantID, normalizedPlate string) (vehicle.Vehicle, error) {
	query := `SELECT ` + vehicleColumns + `
		FROM vehicles
		WHERE tenant_id = $1::uuid AND plate_normalized = $2`

	found, err := scanVehicle(s.pool.QueryRow(ctx, query, tenantID, normalizedPlate))
	if errors.Is(err, pgx.ErrNoRows) {
		return vehicle.Vehicle{}, &domain.NotFoundError{Entity: "vehicle"}
	}
	if err != nil {
		return vehicle.Vehicle{}, fmt.Errorf("postgres: find vehicle by plate: %w", err)
	}
	return found, nil
}

func (s *Store) ListVehiclesByCustomer(ctx context.Context, tenantID, customerID string) ([]vehicle.Vehicle, error) {
	var customerExists bool
	const customerQuery = `
		SELECT EXISTS (
			SELECT 1 FROM customers
			WHERE tenant_id = $1::uuid AND id = $2::uuid
		)`
	if err := s.pool.QueryRow(ctx, customerQuery, tenantID, customerID).Scan(&customerExists); err != nil {
		return nil, fmt.Errorf("postgres: verify customer before listing vehicles: %w", err)
	}
	if !customerExists {
		return nil, &domain.NotFoundError{Entity: "customer", ID: customerID}
	}

	query := `SELECT ` + vehicleColumns + `
		FROM vehicles
		WHERE tenant_id = $1::uuid AND customer_id = $2::uuid
		ORDER BY created_at, id`

	rows, err := s.pool.Query(ctx, query, tenantID, customerID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list vehicles by customer: %w", err)
	}
	defer rows.Close()

	vehicles := make([]vehicle.Vehicle, 0)
	for rows.Next() {
		value, scanErr := scanVehicle(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres: scan vehicle: %w", scanErr)
		}
		vehicles = append(vehicles, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list vehicles by customer rows: %w", err)
	}
	return vehicles, nil
}

func scanVehicle(row pgx.Row) (vehicle.Vehicle, error) {
	var value vehicle.Vehicle
	err := row.Scan(
		&value.ID,
		&value.TenantID,
		&value.CustomerID,
		&value.Plate,
		&value.NormalizedPlate,
		&value.Make,
		&value.Model,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	return value, err
}
