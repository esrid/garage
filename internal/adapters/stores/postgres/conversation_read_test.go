package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

func TestConversationHistoryReadModelUsesTenantDayAndIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}
	ctx := context.Background()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	tenantService := tenant.NewService(store)
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage history A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage history B"})
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	t.Cleanup(func() {
		for _, tenantID := range []string{tenantA.ID, tenantB.ID} {
			if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantID); cleanupErr != nil {
				t.Errorf("cleanup tenant %s: %v", tenantID, cleanupErr)
			}
		}
	})

	location, err := time.LoadLocation("America/Martinique")
	if err != nil {
		t.Fatal(err)
	}
	writeService := conversation.NewService(store)
	record := func(tenantID, providerID string, started time.Time) conversation.RecordResult {
		t.Helper()
		input := postgresConversationInput("agent_history", providerID, started.Add(time.Minute).Unix(), 60, `{"provider_id":"`+providerID+`"}`)
		input.StartedAt = started.UTC()
		input.Transcript = []byte(`[{"role":"user","message":"Bonjour","time_in_call_secs":1}]`)
		result, recordErr := writeService.RecordPostCall(tenant.WithID(ctx, tenantID), input)
		if recordErr != nil {
			t.Fatalf("record %s: %v", providerID, recordErr)
		}
		return result
	}
	localMorning := time.Date(2026, 7, 30, 9, 0, 0, 0, location)
	localAfternoon := time.Date(2026, 7, 30, 16, 0, 0, 0, location)
	older := record(tenantA.ID, "history_morning", localMorning)
	newer := record(tenantA.ID, "history_afternoon", localAfternoon)
	record(tenantA.ID, "history_previous_day", localMorning.AddDate(0, 0, -1))
	foreign := record(tenantB.ID, "history_foreign", localAfternoon)

	reader := conversation.NewHistoryService(store)
	ctxA := tenant.WithID(ctx, tenantA.ID)
	history, err := reader.Day(ctxA, localMorning.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Day() error = %v", err)
	}
	if history.Timezone != location.String() || !history.Date.Equal(time.Date(2026, 7, 30, 0, 0, 0, 0, location)) {
		t.Fatalf("history day = %#v", history)
	}
	if len(history.Conversations) != 2 || history.Conversations[0].ID != newer.Conversation.ID || history.Conversations[1].ID != older.Conversation.ID {
		t.Fatalf("history conversations = %#v", history.Conversations)
	}

	detail, err := reader.Call(ctxA, older.Conversation.ID)
	if err != nil || detail.Timezone != location.String() || detail.Conversation.TenantID != tenantA.ID {
		t.Fatalf("Call() = %#v, %v", detail, err)
	}
	for _, id := range []string{foreign.Conversation.ID, "not-a-uuid"} {
		_, err := reader.Call(ctxA, id)
		var notFound *domain.NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Call(%q) error = %v, want NotFoundError", id, err)
		}
	}
}
