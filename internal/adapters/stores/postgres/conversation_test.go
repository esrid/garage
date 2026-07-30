package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/tenant"
)

func TestPostCallConversationTenantIsolationAndIdempotence(t *testing.T) {
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
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage conversation A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage conversation B"})
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

	service := conversation.NewService(store)
	input := postgresConversationInput("agent_a", "conv_shared", 1739537297, 22, `{"delivery":1}`)
	created, err := service.RecordPostCall(tenant.WithID(ctx, tenantA.ID), input)
	if err != nil {
		t.Fatalf("record post-call: %v", err)
	}
	if created.Duplicate || created.Conversation.TenantID != tenantA.ID || created.Conversation.DurationSeconds != 22 || created.Conversation.CostFiatMicroUSD == nil || *created.Conversation.CostFiatMicroUSD != 1_100_000 {
		t.Fatalf("created = %#v", created)
	}

	replayed, err := service.RecordPostCall(tenant.WithID(ctx, tenantA.ID), input)
	if err != nil || !replayed.Duplicate || replayed.Conversation.ID != created.Conversation.ID {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}

	conflicting := input
	conflicting.RawPayload = []byte(`{"delivery":"changed"}`)
	if _, err := service.RecordPostCall(tenant.WithID(ctx, tenantA.ID), conflicting); !errors.Is(err, conversation.ErrEventConflict) {
		t.Fatalf("changed replay error = %v, want ErrEventConflict", err)
	}

	later := postgresConversationInput("agent_a", "conv_shared", 1739537397, 42, `{"delivery":2}`)
	updated, err := service.RecordPostCall(tenant.WithID(ctx, tenantA.ID), later)
	if err != nil || updated.Conversation.ID != created.Conversation.ID || updated.Conversation.DurationSeconds != 42 {
		t.Fatalf("later event = %#v, %v", updated, err)
	}

	older := postgresConversationInput("agent_a", "conv_shared", 1739537197, 10, `{"delivery":0}`)
	retained, err := service.RecordPostCall(tenant.WithID(ctx, tenantA.ID), older)
	if err != nil || retained.Conversation.DurationSeconds != 42 || !retained.Conversation.ProviderEventAt.Equal(later.ProviderEventAt) {
		t.Fatalf("older event overwrote snapshot: %#v, %v", retained, err)
	}

	otherTenant, err := service.RecordPostCall(tenant.WithID(ctx, tenantB.ID), input)
	if err != nil {
		t.Fatalf("other tenant record: %v", err)
	}
	if otherTenant.Conversation.TenantID != tenantB.ID || otherTenant.Conversation.ID == created.Conversation.ID {
		t.Fatalf("other tenant conversation = %#v", otherTenant)
	}

	var eventCount, conversationCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_events WHERE tenant_id = $1::uuid`, tenantA.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversations WHERE tenant_id = $1::uuid`, tenantA.ID).Scan(&conversationCount); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if eventCount != 3 || conversationCount != 1 {
		t.Fatalf("events=%d conversations=%d, want 3 and 1", eventCount, conversationCount)
	}
}

func TestPostCallConversationConcurrentDeliveries(t *testing.T) {
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
	tenantValue, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Garage conversation concurrency"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantValue.ID); cleanupErr != nil {
			t.Errorf("cleanup tenant: %v", cleanupErr)
		}
	})

	service := conversation.NewService(store)
	tenantCtx := tenant.WithID(ctx, tenantValue.ID)
	input := postgresConversationInput("agent_concurrent", "conv_concurrent", 1739537297, 22, `{"delivery":"same"}`)
	const workers = 8
	results := make(chan conversation.RecordResult, workers)
	errorsFound := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, recordErr := service.RecordPostCall(tenantCtx, input)
			results <- result
			errorsFound <- recordErr
		}()
	}
	group.Wait()
	close(results)
	close(errorsFound)
	for recordErr := range errorsFound {
		if recordErr != nil {
			t.Fatalf("concurrent exact delivery: %v", recordErr)
		}
	}
	var firstID string
	createdCount := 0
	for result := range results {
		if firstID == "" {
			firstID = result.Conversation.ID
		}
		if result.Conversation.ID == "" || result.Conversation.ID != firstID {
			t.Fatalf("concurrent conversation ID=%q, want %q", result.Conversation.ID, firstID)
		}
		if !result.Duplicate {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created deliveries=%d, want 1", createdCount)
	}

	inputs := []conversation.RecordInput{
		postgresConversationInput("agent_concurrent", "conv_conflicting", 1739537397, 30, `{"delivery":"a"}`),
		postgresConversationInput("agent_concurrent", "conv_conflicting", 1739537397, 30, `{"delivery":"b"}`),
	}
	conflictErrors := make(chan error, len(inputs))
	for _, candidate := range inputs {
		group.Add(1)
		go func() {
			defer group.Done()
			_, recordErr := service.RecordPostCall(tenantCtx, candidate)
			conflictErrors <- recordErr
		}()
	}
	group.Wait()
	close(conflictErrors)
	successes, conflicts := 0, 0
	for recordErr := range conflictErrors {
		switch {
		case recordErr == nil:
			successes++
		case errors.Is(recordErr, conversation.ErrEventConflict):
			conflicts++
		default:
			t.Fatalf("unexpected conflicting delivery error: %v", recordErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("conflicting deliveries successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}
}

func postgresConversationInput(agentID, conversationID string, eventUnix int64, duration int, raw string) conversation.RecordInput {
	cost := int64(1_100_000)
	return conversation.RecordInput{
		AgentID:                agentID,
		ProviderConversationID: conversationID,
		ProviderStatus:         "done",
		ProviderEventAt:        time.Unix(eventUnix, 0).UTC(),
		StartedAt:              time.Unix(1739537275, 0).UTC(),
		DurationSeconds:        duration,
		CostFiatMicroUSD:       &cost,
		Transcript:             []byte(`[{"role":"user","message":"Bonjour"}]`),
		Summary:                "Résumé provider",
		ProviderOutcome:        "success",
		Analysis:               []byte(`{"call_successful":"success"}`),
		Metadata:               []byte(`{"call_duration_secs":22,"cost_fiat":1.1}`),
		RawPayload:             []byte(raw),
	}
}
