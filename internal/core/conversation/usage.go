package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

// The PRD's economic rule (§5): no unlimited plan, a quota of voice minutes per
// month, and a warning at 70, 85 and 100 % of it. Until now a workshop had no way
// to know where it stood, and the promise lived only on the pricing page.

// Alert thresholds, as percentages of the monthly quota.
const (
	AlertNoticeAt  = 70
	AlertWarningAt = 85
	AlertOverAt    = 100
)

// Usage is one workshop's consumption over one month.
type Usage struct {
	// Month is the first instant of the month, in the tenant timezone.
	Month    time.Time
	Timezone string
	// Calls is how many conversations were recorded.
	Calls int
	// Seconds is the total call duration. Minutes are derived rather than stored,
	// so rounding happens once, where it is displayed.
	Seconds int
	// QuotaMinutes is what the workshop bought for the month.
	QuotaMinutes int
}

// UsageStore is the persistence capability this read needs.
type UsageStore interface {
	ConversationUsage(ctx context.Context, tenantID string, month time.Time) (Usage, error)
}

// UsageReader is what the HTTP adapter consumes.
type UsageReader interface {
	Usage(ctx context.Context, month time.Time) (Usage, error)
}

type UsageService struct {
	store UsageStore
}

func NewUsageService(store UsageStore) *UsageService {
	return &UsageService{store: store}
}

func (s *UsageService) Usage(ctx context.Context, month time.Time) (Usage, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Usage{}, err
	}
	if month.IsZero() {
		return Usage{}, &domain.ValidationError{Entity: "usage", Errors: map[string]string{"month": "is required"}}
	}
	usage, err := s.store.ConversationUsage(ctx, tenantID, month)
	if err != nil {
		return Usage{}, err
	}
	if usage.QuotaMinutes <= 0 {
		return Usage{}, fmt.Errorf("usage store returned a tenant without a quota")
	}
	return usage, nil
}

// Minutes rounds up: a 30-second call consumes a minute of the plan, which is how
// a customer reads a phone bill and the only rounding that cannot undercount.
func (u Usage) Minutes() int {
	return (u.Seconds + 59) / 60
}

// Percent is the share of the quota consumed, uncapped: going over is a fact the
// workshop needs to see, not one to hide behind a full bar.
func (u Usage) Percent() int {
	if u.QuotaMinutes <= 0 {
		return 0
	}
	return u.Minutes() * 100 / u.QuotaMinutes
}

// Alert is the threshold crossed: 0, 70, 85 or 100.
func (u Usage) Alert() int {
	switch percent := u.Percent(); {
	case percent >= AlertOverAt:
		return AlertOverAt
	case percent >= AlertWarningAt:
		return AlertWarningAt
	case percent >= AlertNoticeAt:
		return AlertNoticeAt
	default:
		return 0
	}
}

// RemainingMinutes never goes below zero: what is left is what can still be used.
func (u Usage) RemainingMinutes() int {
	return max(u.QuotaMinutes-u.Minutes(), 0)
}
