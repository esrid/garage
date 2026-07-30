package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/web/views"
)

// TodayWithFollowUpsProvider fills the dashboard's "à traiter" panel with the
// pending callback and quote requests the voice agent recorded.
//
// A wrapper rather than a wider provider: each layer maps one domain into the
// frozen F04 DTO, and the composition root decides which ones are in play.
type TodayWithFollowUpsProvider struct {
	base      TodayProvider
	followUps followup.PendingReader
}

func NewTodayWithFollowUpsProvider(base TodayProvider, followUps followup.PendingReader) *TodayWithFollowUpsProvider {
	return &TodayWithFollowUpsProvider{base: base, followUps: followUps}
}

func (p *TodayWithFollowUpsProvider) Today(ctx context.Context, day time.Time) (views.Today, error) {
	result, err := p.base.Today(ctx, day)
	if err != nil {
		return views.Today{}, err
	}
	pending, err := p.followUps.Pending(ctx)
	if err != nil {
		return views.Today{}, fmt.Errorf("dashboard follow-ups: %w", err)
	}

	result.Tasks = make([]views.Task, 0, len(pending))
	for _, request := range pending {
		result.Tasks = append(result.Tasks, views.Task{
			ID:        request.ID,
			CreatedAt: request.CreatedAt,
			Kind:      string(request.Kind),
			// Both may be empty; the view titles the row with whichever it has and
			// never invents a name (PRD 7.1).
			CustomerName: request.CustomerName,
			Phone:        request.Phone,
			Note:         request.Details,
		})
	}
	return result, nil
}
