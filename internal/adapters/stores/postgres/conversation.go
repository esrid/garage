package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/jackc/pgx/v5"
)

var _ conversation.Store = (*Store)(nil)

const conversationColumns = `
	id::text, tenant_id::text, provider, agent_id, provider_conversation_id,
	provider_status, provider_event_at, started_at, duration_seconds,
	cost_fiat_microusd, transcript, COALESCE(summary, ''),
	COALESCE(provider_outcome, ''), analysis, metadata, created_at, updated_at`

func (s *Store) RecordPostCall(ctx context.Context, tenantID string, input conversation.RecordInput, payloadHash [sha256.Size]byte) (result conversation.RecordResult, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("postgres: begin post-call event: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("postgres: rollback post-call event: %w", rollbackErr))
		}
	}()

	const insertEvent = `
		INSERT INTO conversation_events (
			tenant_id, provider, event_type, provider_conversation_id,
			provider_event_at, payload_hash, raw_payload
		) VALUES (
			$1::uuid, 'elevenlabs', 'post_call_transcription', $2, $3, $4, $5::jsonb
		)
		ON CONFLICT (
			tenant_id, provider, event_type, provider_conversation_id, provider_event_at
		) DO NOTHING
		RETURNING id::text`
	var eventID string
	err = tx.QueryRow(ctx, insertEvent,
		tenantID, input.ProviderConversationID, input.ProviderEventAt,
		payloadHash[:], input.RawPayload,
	).Scan(&eventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, fmt.Errorf("postgres: insert post-call event: %w", err)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		const existingHashQuery = `
			SELECT payload_hash
			FROM conversation_events
			WHERE tenant_id = $1::uuid
			  AND provider = 'elevenlabs'
			  AND event_type = 'post_call_transcription'
			  AND provider_conversation_id = $2
			  AND provider_event_at = $3`
		var existingHash []byte
		if err := tx.QueryRow(ctx, existingHashQuery, tenantID, input.ProviderConversationID, input.ProviderEventAt).Scan(&existingHash); err != nil {
			return result, fmt.Errorf("postgres: load post-call event hash: %w", err)
		}
		if !bytes.Equal(existingHash, payloadHash[:]) {
			return result, conversation.ErrEventConflict
		}
		result.Conversation, err = loadConversation(ctx, tx, tenantID, input.ProviderConversationID)
		if err != nil {
			return conversation.RecordResult{}, err
		}
		result.Duplicate = true
		if err := tx.Commit(ctx); err != nil {
			return conversation.RecordResult{}, fmt.Errorf("postgres: commit duplicate post-call event: %w", err)
		}
		return result, nil
	}

	const upsertConversation = `
		INSERT INTO conversations (
			tenant_id, provider, agent_id, provider_conversation_id,
			provider_status, provider_event_at, started_at, duration_seconds,
			cost_fiat_microusd, transcript, summary, provider_outcome,
			analysis, metadata
		) VALUES (
			$1::uuid, 'elevenlabs', $2, $3, $4, $5, $6, $7, $8,
			$9::jsonb, NULLIF($10, ''), NULLIF($11, ''), $12::jsonb, $13::jsonb
		)
		ON CONFLICT (tenant_id, provider, provider_conversation_id) DO UPDATE SET
			agent_id = EXCLUDED.agent_id,
			provider_status = EXCLUDED.provider_status,
			provider_event_at = EXCLUDED.provider_event_at,
			started_at = EXCLUDED.started_at,
			duration_seconds = EXCLUDED.duration_seconds,
			cost_fiat_microusd = EXCLUDED.cost_fiat_microusd,
			transcript = EXCLUDED.transcript,
			summary = EXCLUDED.summary,
			provider_outcome = EXCLUDED.provider_outcome,
			analysis = EXCLUDED.analysis,
			metadata = EXCLUDED.metadata,
			updated_at = now()
		WHERE conversations.provider_event_at <= EXCLUDED.provider_event_at
		RETURNING ` + conversationColumns
	result.Conversation, err = scanConversation(tx.QueryRow(ctx, upsertConversation,
		tenantID, input.AgentID, input.ProviderConversationID, input.ProviderStatus,
		input.ProviderEventAt, input.StartedAt, input.DurationSeconds,
		input.CostFiatMicroUSD, input.Transcript, input.Summary,
		input.ProviderOutcome, input.Analysis, input.Metadata,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		result.Conversation, err = loadConversation(ctx, tx, tenantID, input.ProviderConversationID)
	}
	if err != nil {
		return conversation.RecordResult{}, fmt.Errorf("postgres: upsert post-call conversation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return conversation.RecordResult{}, fmt.Errorf("postgres: commit post-call event: %w", err)
	}
	return result, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadConversation(ctx context.Context, db queryRower, tenantID, providerConversationID string) (conversation.Conversation, error) {
	const query = `SELECT ` + conversationColumns + `
		FROM conversations
		WHERE tenant_id = $1::uuid
		  AND provider = 'elevenlabs'
		  AND provider_conversation_id = $2`
	value, err := scanConversation(db.QueryRow(ctx, query, tenantID, providerConversationID))
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("postgres: load post-call conversation: %w", err)
	}
	return value, nil
}

func scanConversation(row pgx.Row) (conversation.Conversation, error) {
	var value conversation.Conversation
	err := row.Scan(
		&value.ID,
		&value.TenantID,
		&value.Provider,
		&value.AgentID,
		&value.ProviderConversationID,
		&value.ProviderStatus,
		&value.ProviderEventAt,
		&value.StartedAt,
		&value.DurationSeconds,
		&value.CostFiatMicroUSD,
		&value.Transcript,
		&value.Summary,
		&value.ProviderOutcome,
		&value.Analysis,
		&value.Metadata,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	return value, err
}
