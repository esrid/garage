package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/esrid/garage/internal/web/views"
)

// TodayWithCallsProvider adds persisted calls to the existing appointment day
// without making either domain depend on presentation DTOs.
type TodayWithCallsProvider struct {
	base  TodayProvider
	calls CallHistoryReader
}

func NewTodayWithCallsProvider(base TodayProvider, calls CallHistoryReader) *TodayWithCallsProvider {
	return &TodayWithCallsProvider{base: base, calls: calls}
}

func (p *TodayWithCallsProvider) Today(ctx context.Context, day time.Time) (views.Today, error) {
	result, err := p.base.Today(ctx, day)
	if err != nil {
		return views.Today{}, err
	}
	history, err := p.calls.Calls(ctx, day)
	if err != nil {
		return views.Today{}, fmt.Errorf("dashboard calls: %w", err)
	}
	result.Calls = make([]views.Call, 0, len(history.Calls))
	for _, call := range history.Calls {
		result.Calls = append(result.Calls, views.Call{
			ID:           call.ID,
			At:           call.At,
			Duration:     call.Duration,
			CustomerName: call.CustomerName,
			Phone:        call.Phone,
			Outcome:      call.Outcome,
			Transferred:  call.Outcome == "transferred",
		})
	}
	return result, nil
}
