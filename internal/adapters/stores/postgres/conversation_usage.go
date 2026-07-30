package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
)

var _ conversation.UsageStore = (*Store)(nil)

// ConversationUsage totals one month of calls for one workshop.
//
// The month boundaries are computed in the tenant's own timezone, in SQL, for the
// same reason every other day-scoped read does it: a month that starts at
// midnight UTC starts on the last day of the previous month in Martinique, and a
// bill would be off by a day at both ends.
func (s *Store) ConversationUsage(ctx context.Context, tenantID string, month time.Time) (conversation.Usage, error) {
	const query = `
		WITH workshop AS (
			SELECT timezone, monthly_minutes_quota
			FROM tenants
			WHERE id = $1::uuid
		), period AS (
			SELECT
				date_trunc('month', ($2 AT TIME ZONE workshop.timezone)) AS month_start,
				workshop.timezone,
				workshop.monthly_minutes_quota
			FROM workshop
		)
		SELECT
			(period.month_start AT TIME ZONE period.timezone) AS month_start,
			period.timezone,
			period.monthly_minutes_quota,
			COALESCE(COUNT(c.id), 0),
			COALESCE(SUM(c.duration_seconds), 0)
		FROM period
		LEFT JOIN conversations c
			ON c.tenant_id = $1::uuid
			AND c.started_at >= (period.month_start AT TIME ZONE period.timezone)
			AND c.started_at < ((period.month_start + interval '1 month') AT TIME ZONE period.timezone)
		GROUP BY period.month_start, period.timezone, period.monthly_minutes_quota`

	var usage conversation.Usage
	if err := s.pool.QueryRow(ctx, query, tenantID, month).Scan(
		&usage.Month, &usage.Timezone, &usage.QuotaMinutes, &usage.Calls, &usage.Seconds,
	); err != nil {
		return conversation.Usage{}, fmt.Errorf("postgres: conversation usage: %w", err)
	}
	return usage, nil
}
