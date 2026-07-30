package postgres

import (
	"context"
	"fmt"

	"github.com/esrid/garage/internal/core/followup"
)

var _ followup.ReadStore = (*Store)(nil)

// PendingFollowUps lists the requests still to handle for one tenant, oldest
// first, with the customer name resolved in the same query.
//
// The LEFT JOIN is what keeps this one round trip: a request created for an
// unknown caller has no customer row, and the name comes back empty rather than
// dropping the row. The partial index follow_up_requests_pending_idx covers the
// filter and the order exactly.
func (s *Store) PendingFollowUps(ctx context.Context, tenantID string, limit int) ([]followup.Pending, error) {
	const query = `
		SELECT f.id::text, f.tenant_id::text, COALESCE(f.customer_id::text, ''), f.kind,
			f.phone_e164, f.details, f.status, f.conversation_id, f.created_at, f.updated_at,
			btrim(concat_ws(' ', c.first_name, c.last_name))
		FROM follow_up_requests f
		LEFT JOIN customers c ON c.tenant_id = f.tenant_id AND c.id = f.customer_id
		WHERE f.tenant_id = $1::uuid AND f.status = 'pending'
		ORDER BY f.created_at, f.id
		LIMIT $2`

	rows, err := s.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list pending follow-ups: %w", err)
	}
	defer rows.Close()

	result := make([]followup.Pending, 0, 16)
	for rows.Next() {
		var pending followup.Pending
		if err := rows.Scan(
			&pending.ID,
			&pending.TenantID,
			&pending.CustomerID,
			&pending.Kind,
			&pending.Phone,
			&pending.Details,
			&pending.Status,
			&pending.ConversationID,
			&pending.CreatedAt,
			&pending.UpdatedAt,
			&pending.CustomerName,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan pending follow-up: %w", err)
		}
		result = append(result, pending)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: pending follow-up rows: %w", err)
	}
	return result, nil
}

// CallersByConversation resolves who called, for the conversations we recorded a
// follow-up request for. A conversation with several requests resolves once:
// they carry the same caller by construction.
func (s *Store) CallersByConversation(ctx context.Context, tenantID string, conversationIDs []string) (map[string]followup.Caller, error) {
	const query = `
		SELECT DISTINCT ON (f.conversation_id)
			f.conversation_id, f.phone_e164,
			btrim(concat_ws(' ', c.first_name, c.last_name))
		FROM follow_up_requests f
		LEFT JOIN customers c ON c.tenant_id = f.tenant_id AND c.id = f.customer_id
		WHERE f.tenant_id = $1::uuid AND f.conversation_id = ANY($2)
		ORDER BY f.conversation_id, f.created_at`

	rows, err := s.pool.Query(ctx, query, tenantID, conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: resolve conversation callers: %w", err)
	}
	defer rows.Close()

	result := make(map[string]followup.Caller, len(conversationIDs))
	for rows.Next() {
		var conversationID string
		var caller followup.Caller
		if err := rows.Scan(&conversationID, &caller.Phone, &caller.CustomerName); err != nil {
			return nil, fmt.Errorf("postgres: scan conversation caller: %w", err)
		}
		result[conversationID] = caller
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: conversation caller rows: %w", err)
	}
	return result, nil
}
