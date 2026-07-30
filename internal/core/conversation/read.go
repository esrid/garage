package conversation

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

// HistoryStore is the narrow persistence contract needed by call history.
// tenantID is always resolved by HistoryService, never accepted from HTTP.
type HistoryStore interface {
	ConversationDay(context.Context, string, time.Time) (HistoryDay, error)
	ConversationByID(context.Context, string, string) (HistoryEntry, error)
}

// HistoryReader is the domain read capability consumed by the HTTP adapter.
type HistoryReader interface {
	Day(context.Context, time.Time) (HistoryDay, error)
	Call(context.Context, string) (HistoryEntry, error)
}

type HistoryService struct {
	store HistoryStore
}

func NewHistoryService(store HistoryStore) *HistoryService {
	return &HistoryService{store: store}
}

func (s *HistoryService) Day(ctx context.Context, day time.Time) (HistoryDay, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return HistoryDay{}, err
	}
	if day.IsZero() {
		return HistoryDay{}, &domain.ValidationError{Entity: "conversation history", Errors: map[string]string{"day": "is required"}}
	}

	result, err := s.store.ConversationDay(ctx, tenantID, day)
	if err != nil {
		return HistoryDay{}, err
	}
	if result.Date.IsZero() || strings.TrimSpace(result.Timezone) == "" {
		return HistoryDay{}, fmt.Errorf("conversation history store returned an invalid day")
	}
	for _, entry := range result.Conversations {
		if entry.TenantID != tenantID || entry.Provider != ProviderElevenLabs {
			return HistoryDay{}, fmt.Errorf("conversation history store returned an invalid conversation")
		}
	}
	return result, nil
}

func (s *HistoryService) Call(ctx context.Context, id string) (HistoryEntry, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return HistoryEntry{}, err
	}
	canonicalID := strings.ToLower(id)
	if !validHistoryID(canonicalID) {
		return HistoryEntry{}, &domain.NotFoundError{Entity: "conversation", ID: id}
	}

	result, err := s.store.ConversationByID(ctx, tenantID, canonicalID)
	if err != nil {
		return HistoryEntry{}, err
	}
	if result.Conversation.ID != canonicalID || result.Conversation.TenantID != tenantID || result.Conversation.Provider != ProviderElevenLabs || strings.TrimSpace(result.Timezone) == "" {
		return HistoryEntry{}, fmt.Errorf("conversation history store returned an invalid result")
	}
	return result, nil
}

func validHistoryID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}
