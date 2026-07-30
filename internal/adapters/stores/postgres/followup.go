package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/esrid/garage/internal/core/followup"
	"github.com/jackc/pgx/v5"
)

var _ followup.Store = (*Store)(nil)

const followUpColumns = `
	id::text, tenant_id::text, COALESCE(customer_id::text, ''), kind,
	phone_e164, details, status, conversation_id, created_at, updated_at`

func (s *Store) CreateFollowUpRequest(ctx context.Context, tenantID string, input followup.CreateInput, requestHash string) (followup.Request, error) {
	const insert = `
		INSERT INTO follow_up_requests (
			tenant_id, customer_id, kind, phone_e164, details,
			conversation_id, request_hash
		)
		VALUES (
			$1::uuid,
			(SELECT id FROM customers WHERE tenant_id = $1::uuid AND phone_e164 = $4),
			$3, $4, $5, $2, $6
		)
		ON CONFLICT (tenant_id, conversation_id, kind) DO NOTHING
		RETURNING ` + followUpColumns

	created, err := scanFollowUpRequest(s.pool.QueryRow(
		ctx,
		insert,
		tenantID,
		input.ConversationID,
		input.Kind,
		input.Phone,
		input.Details,
		requestHash,
	))
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return followup.Request{}, fmt.Errorf("postgres: create follow-up request: %w", err)
	}

	const existingQuery = `SELECT ` + followUpColumns + `, request_hash
		FROM follow_up_requests
		WHERE tenant_id = $1::uuid AND conversation_id = $2 AND kind = $3`
	var existing followup.Request
	var existingHash string
	err = s.pool.QueryRow(ctx, existingQuery, tenantID, input.ConversationID, input.Kind).Scan(
		&existing.ID,
		&existing.TenantID,
		&existing.CustomerID,
		&existing.Kind,
		&existing.Phone,
		&existing.Details,
		&existing.Status,
		&existing.ConversationID,
		&existing.CreatedAt,
		&existing.UpdatedAt,
		&existingHash,
	)
	if err != nil {
		return followup.Request{}, fmt.Errorf("postgres: load existing follow-up request: %w", err)
	}
	if existingHash != requestHash {
		return followup.Request{}, followup.ErrIdempotencyConflict
	}
	return existing, nil
}

func scanFollowUpRequest(row pgx.Row) (followup.Request, error) {
	var value followup.Request
	err := row.Scan(
		&value.ID,
		&value.TenantID,
		&value.CustomerID,
		&value.Kind,
		&value.Phone,
		&value.Details,
		&value.Status,
		&value.ConversationID,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	return value, err
}
