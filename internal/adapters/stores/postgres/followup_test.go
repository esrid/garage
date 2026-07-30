package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/core/tenant"
)

func TestFollowUpRequestTenantIsolationAndIdempotence(t *testing.T) {
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
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage follow-up A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage follow-up B"})
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

	ctxA := tenant.WithID(ctx, tenantA.ID)
	ctxB := tenant.WithID(ctx, tenantB.ID)
	known, err := customer.NewService(store).Create(ctxA, customer.CreateInput{
		FirstName: "Ana",
		Phone:     "+596696100001",
	})
	if err != nil {
		t.Fatalf("create known customer: %v", err)
	}

	service := followup.NewService(store)
	input := followup.CreateInput{
		ConversationID: "conv_followup_1",
		Kind:           followup.KindCallback,
		Phone:          "+596 696 10 00 01",
		Details:        "Rappeler pour préciser la demande.",
	}
	created, err := service.Create(ctxA, input)
	if err != nil {
		t.Fatalf("create follow-up: %v", err)
	}
	if created.TenantID != tenantA.ID || created.CustomerID != known.ID || created.Status != followup.StatusPending || created.Phone != "+596696100001" {
		t.Fatalf("created follow-up = %#v", created)
	}

	replayed, err := service.Create(ctxA, input)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replayed follow-up = %#v, %v; want ID %s", replayed, err, created.ID)
	}

	changed := input
	changed.Details = "Contenu différent."
	_, err = service.Create(ctxA, changed)
	if !errors.Is(err, followup.ErrIdempotencyConflict) {
		t.Fatalf("changed replay error = %v, want ErrIdempotencyConflict", err)
	}

	quote := input
	quote.Kind = followup.KindQuote
	quoted, err := service.Create(ctxA, quote)
	if err != nil || quoted.ID == created.ID {
		t.Fatalf("quote follow-up = %#v, %v", quoted, err)
	}

	otherTenant, err := service.Create(ctxB, input)
	if err != nil {
		t.Fatalf("create other-tenant follow-up: %v", err)
	}
	if otherTenant.TenantID != tenantB.ID || otherTenant.ID == created.ID || otherTenant.CustomerID != "" {
		t.Fatalf("other tenant follow-up = %#v", otherTenant)
	}
}

func TestFollowUpRequestConcurrentReplayReturnsOneRow(t *testing.T) {
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
	tenantValue, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Garage follow-up concurrency"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantValue.ID); cleanupErr != nil {
			t.Errorf("cleanup tenant: %v", cleanupErr)
		}
	})

	service := followup.NewService(store)
	tenantCtx := tenant.WithID(ctx, tenantValue.ID)
	input := followup.CreateInput{
		ConversationID: "conv_concurrent",
		Kind:           followup.KindCallback,
		Phone:          "+596696100002",
		Details:        "Rappel concurrent.",
	}
	const workers = 8
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			created, createErr := service.Create(tenantCtx, input)
			ids <- created.ID
			errs <- createErr
		}()
	}
	group.Wait()
	close(ids)
	close(errs)
	for createErr := range errs {
		if createErr != nil {
			t.Fatalf("concurrent Create() error = %v", createErr)
		}
	}
	var firstID string
	for id := range ids {
		if firstID == "" {
			firstID = id
		}
		if id == "" || id != firstID {
			t.Fatalf("concurrent IDs include %q, want only %q", id, firstID)
		}
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM follow_up_requests
		WHERE tenant_id = $1::uuid AND conversation_id = $2 AND kind = $3`,
		tenantValue.ID, input.ConversationID, input.Kind,
	).Scan(&count); err != nil || count != 1 {
		t.Fatalf("persisted count=%d error=%v, want 1", count, err)
	}
}

func TestFollowUpRequestConcurrentDifferentContentConflicts(t *testing.T) {
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
	tenantValue, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Garage follow-up conflicting concurrency"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantValue.ID); cleanupErr != nil {
			t.Errorf("cleanup tenant: %v", cleanupErr)
		}
	})

	service := followup.NewService(store)
	tenantCtx := tenant.WithID(ctx, tenantValue.ID)
	inputs := []followup.CreateInput{
		{ConversationID: "conv_conflict", Kind: followup.KindQuote, Phone: "+596696100003", Details: "Demande A."},
		{ConversationID: "conv_conflict", Kind: followup.KindQuote, Phone: "+596696100003", Details: "Demande B."},
	}
	results := make(chan error, len(inputs))
	var group sync.WaitGroup
	for _, input := range inputs {
		group.Add(1)
		go func() {
			defer group.Done()
			_, createErr := service.Create(tenantCtx, input)
			results <- createErr
		}()
	}
	group.Wait()
	close(results)
	successes := 0
	conflicts := 0
	for createErr := range results {
		switch {
		case createErr == nil:
			successes++
		case errors.Is(createErr, followup.ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error = %v", createErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results: successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}
}
