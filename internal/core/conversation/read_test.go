package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type historyStoreStub struct {
	day  func(context.Context, string, time.Time) (HistoryDay, error)
	call func(context.Context, string, string) (HistoryEntry, error)
}

func (s historyStoreStub) ConversationDay(ctx context.Context, tenantID string, day time.Time) (HistoryDay, error) {
	return s.day(ctx, tenantID, day)
}

func (s historyStoreStub) ConversationByID(ctx context.Context, tenantID, id string) (HistoryEntry, error) {
	return s.call(ctx, tenantID, id)
}

func TestHistoryServiceResolvesTenantForDayAndCall(t *testing.T) {
	day := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	id := "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1"
	store := historyStoreStub{
		day: func(_ context.Context, tenantID string, gotDay time.Time) (HistoryDay, error) {
			if tenantID != conversationTenantID || !gotDay.Equal(day) {
				t.Fatalf("ConversationDay tenant=%q day=%v", tenantID, gotDay)
			}
			return HistoryDay{
				Date:     day,
				Timezone: "America/Martinique",
				Conversations: []Conversation{{
					ID: id, TenantID: tenantID, Provider: ProviderElevenLabs,
				}},
			}, nil
		},
		call: func(_ context.Context, tenantID, gotID string) (HistoryEntry, error) {
			if tenantID != conversationTenantID || gotID != id {
				t.Fatalf("ConversationByID tenant=%q id=%q", tenantID, gotID)
			}
			return HistoryEntry{
				Conversation: Conversation{ID: id, TenantID: tenantID, Provider: ProviderElevenLabs},
				Timezone:     "America/Martinique",
			}, nil
		},
	}
	service := NewHistoryService(store)
	ctx := tenant.WithID(context.Background(), conversationTenantID)
	if _, err := service.Day(ctx, day); err != nil {
		t.Fatalf("Day() error = %v", err)
	}
	if _, err := service.Call(ctx, id); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if _, err := service.Call(ctx, strings.ToUpper(id)); err != nil {
		t.Fatalf("Call() uppercase canonical ID error = %v", err)
	}
}

func TestHistoryServiceRejectsMissingTenantInvalidIDAndCrossTenantResults(t *testing.T) {
	called := false
	store := historyStoreStub{
		day: func(context.Context, string, time.Time) (HistoryDay, error) {
			called = true
			return HistoryDay{}, nil
		},
		call: func(_ context.Context, _, id string) (HistoryEntry, error) {
			called = true
			return HistoryEntry{
				Conversation: Conversation{ID: id, TenantID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea2", Provider: ProviderElevenLabs},
				Timezone:     "America/Martinique",
			}, nil
		},
	}
	service := NewHistoryService(store)
	if _, err := service.Day(context.Background(), time.Now()); err == nil {
		t.Fatal("Day without tenant succeeded")
	}
	ctx := tenant.WithID(context.Background(), conversationTenantID)
	if _, err := service.Call(ctx, "not-a-uuid"); err == nil {
		t.Fatal("invalid call ID succeeded")
	} else {
		var notFound *domain.NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("invalid ID error = %v, want NotFoundError", err)
		}
	}
	if called {
		t.Fatal("request rejected before store was expected")
	}

	id := "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1"
	if _, err := service.Call(ctx, id); err == nil {
		t.Fatal("cross-tenant store result succeeded")
	}
}
