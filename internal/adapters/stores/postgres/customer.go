package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ customer.Store = (*Store)(nil)

const customerColumns = `
	id::text, tenant_id::text, first_name, last_name, phone_e164,
	created_at, updated_at`

func (s *Store) CreateCustomer(ctx context.Context, tenantID string, input customer.CreateInput) (customer.Customer, error) {
	query := `
		INSERT INTO customers (tenant_id, first_name, last_name, phone_e164)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING ` + customerColumns

	created, err := scanCustomer(s.pool.QueryRow(ctx, query, tenantID, input.FirstName, input.LastName, input.Phone))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "customers_tenant_phone_key" {
			return customer.Customer{}, &domain.AlreadyExistsError{Entity: "customer", Field: "phone", Value: input.Phone}
		}
		return customer.Customer{}, fmt.Errorf("postgres: create customer: %w", err)
	}
	return created, nil
}

func (s *Store) FindCustomerByPhone(ctx context.Context, tenantID, normalizedPhone string) (customer.Customer, error) {
	query := `SELECT ` + customerColumns + `
		FROM customers
		WHERE tenant_id = $1::uuid AND phone_e164 = $2`

	found, err := scanCustomer(s.pool.QueryRow(ctx, query, tenantID, normalizedPhone))
	if errors.Is(err, pgx.ErrNoRows) {
		return customer.Customer{}, &domain.NotFoundError{Entity: "customer"}
	}
	if err != nil {
		return customer.Customer{}, fmt.Errorf("postgres: find customer by phone: %w", err)
	}
	return found, nil
}

func scanCustomer(row pgx.Row) (customer.Customer, error) {
	var value customer.Customer
	err := row.Scan(
		&value.ID,
		&value.TenantID,
		&value.FirstName,
		&value.LastName,
		&value.Phone,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	return value, err
}
