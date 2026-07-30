package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ coreauth.Store = (*Store)(nil)

const staffColumns = `
	id::text, tenant_id::text, email, display_name, role, disabled, created_at, updated_at`

func (s *Store) CreateStaff(ctx context.Context, tenantID string, record coreauth.CreateStaffRecord) (coreauth.Staff, error) {
	query := `
		INSERT INTO staff_users (tenant_id, email, display_name, role, password_hash)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING ` + staffColumns

	created, err := scanStaff(s.pool.QueryRow(ctx, query, tenantID, record.Email, record.DisplayName, record.Role, record.PasswordHash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "staff_users_email_key" {
			return coreauth.Staff{}, &domain.AlreadyExistsError{Entity: "staff", Field: "email", Value: record.Email}
		}
		return coreauth.Staff{}, fmt.Errorf("postgres: create staff: %w", err)
	}
	return created, nil
}

func (s *Store) FindStaffByEmail(ctx context.Context, email string) (coreauth.Credentials, error) {
	query := `SELECT ` + staffColumns + `, password_hash, failed_login_attempts, locked_until FROM staff_users WHERE email = $1`
	found, err := scanCredentials(s.pool.QueryRow(ctx, query, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return coreauth.Credentials{}, &domain.NotFoundError{Entity: "staff"}
	}
	if err != nil {
		return coreauth.Credentials{}, fmt.Errorf("postgres: find staff by email: %w", err)
	}
	return found, nil
}

func (s *Store) RecordLoginFailure(ctx context.Context, staffID string, now, lockUntil time.Time, limit int) error {
	const query = `
		UPDATE staff_users
		SET failed_login_attempts = CASE
		        WHEN locked_until IS NOT NULL AND locked_until <= $2::timestamptz THEN 1
		        ELSE failed_login_attempts + 1
		    END,
		    locked_until = CASE
		        WHEN (CASE
		            WHEN locked_until IS NOT NULL AND locked_until <= $2::timestamptz THEN 1
		            ELSE failed_login_attempts + 1
		        END) >= $3::integer THEN $4::timestamptz
		        ELSE NULL
		    END,
		    updated_at = $2::timestamptz
		WHERE id = $1::uuid`
	tag, err := s.pool.Exec(ctx, query, staffID, now, limit, lockUntil)
	if err != nil {
		return fmt.Errorf("postgres: record login failure: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return &domain.NotFoundError{Entity: "staff", ID: staffID}
	}
	return nil
}

func (s *Store) ClearLoginFailures(ctx context.Context, staffID string) error {
	const query = `
		UPDATE staff_users
		SET failed_login_attempts = 0, locked_until = NULL, updated_at = now()
		WHERE id = $1::uuid`
	tag, err := s.pool.Exec(ctx, query, staffID)
	if err != nil {
		return fmt.Errorf("postgres: clear login failures: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return &domain.NotFoundError{Entity: "staff", ID: staffID}
	}
	return nil
}

func (s *Store) CreateBrowserSession(ctx context.Context, tokenHash []byte, identity coreauth.Identity, expiresAt time.Time) error {
	const query = `
		INSERT INTO browser_sessions (token_hash, tenant_id, staff_user_id, expires_at)
		VALUES ($1, $2::uuid, $3::uuid, $4)`
	if _, err := s.pool.Exec(ctx, query, tokenHash, identity.TenantID, identity.StaffID, expiresAt); err != nil {
		return fmt.Errorf("postgres: create browser session: %w", err)
	}
	return nil
}

func (s *Store) FindBrowserSession(ctx context.Context, tokenHash []byte, now time.Time) (coreauth.Identity, error) {
	const query = `
		SELECT u.id::text, u.tenant_id::text, u.email, u.display_name, u.role
		FROM browser_sessions s
		JOIN staff_users u
		  ON u.tenant_id = s.tenant_id AND u.id = s.staff_user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2 AND NOT u.disabled`
	var identity coreauth.Identity
	err := s.pool.QueryRow(ctx, query, tokenHash, now).Scan(
		&identity.StaffID,
		&identity.TenantID,
		&identity.Email,
		&identity.DisplayName,
		&identity.Role,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return coreauth.Identity{}, &domain.NotFoundError{Entity: "session"}
	}
	if err != nil {
		return coreauth.Identity{}, fmt.Errorf("postgres: find browser session: %w", err)
	}
	return identity, nil
}

func (s *Store) DeleteBrowserSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM browser_sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("postgres: delete browser session: %w", err)
	}
	return nil
}

func scanStaff(row pgx.Row) (coreauth.Staff, error) {
	var value coreauth.Staff
	err := row.Scan(
		&value.ID,
		&value.TenantID,
		&value.Email,
		&value.DisplayName,
		&value.Role,
		&value.Disabled,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	return value, err
}

func scanCredentials(row pgx.Row) (coreauth.Credentials, error) {
	var value coreauth.Credentials
	err := row.Scan(
		&value.Staff.ID,
		&value.Staff.TenantID,
		&value.Staff.Email,
		&value.Staff.DisplayName,
		&value.Staff.Role,
		&value.Staff.Disabled,
		&value.Staff.CreatedAt,
		&value.Staff.UpdatedAt,
		&value.PasswordHash,
		&value.FailedLoginAttempts,
		&value.LockedUntil,
	)
	return value, err
}
