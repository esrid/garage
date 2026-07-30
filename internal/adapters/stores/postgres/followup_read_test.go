package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/core/tenant"
)

// The pending list is what the dashboard's "à traiter" panel shows, so it has to
// be tenant-scoped, ordered oldest first, and carry the customer name when the
// caller is known — all of it verified against a real PostgreSQL.
func TestPendingFollowUpsAreTenantScopedAndOrdered(t *testing.T) {
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
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage pending A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage pending B"})
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
	requests := followup.NewService(store)

	if _, err := customer.NewService(store).Create(ctxA, customer.CreateInput{
		FirstName: "Ana", LastName: "Bertrand", Phone: "+596696200001",
	}); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	// Known caller first, then an unknown one, then another tenant's request.
	if _, err := requests.Create(ctxA, followup.CreateInput{
		ConversationID: "conv_pending_1", Kind: followup.KindCallback,
		Phone: "+596696200001", Details: "Rappeler pour l'embrayage",
	}); err != nil {
		t.Fatalf("create known-caller request: %v", err)
	}
	if _, err := requests.Create(ctxA, followup.CreateInput{
		ConversationID: "conv_pending_2", Kind: followup.KindQuote,
		Phone: "+596696200002", Details: "Devis plaquettes",
	}); err != nil {
		t.Fatalf("create unknown-caller request: %v", err)
	}
	if _, err := requests.Create(ctxB, followup.CreateInput{
		ConversationID: "conv_pending_3", Kind: followup.KindCallback,
		Phone: "+596696200003", Details: "Autre atelier",
	}); err != nil {
		t.Fatalf("create other tenant request: %v", err)
	}

	pending, err := followup.NewReadService(store).Pending(ctxA)
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("got %d pending requests, want 2: the other tenant's must not appear", len(pending))
	}
	if pending[0].ConversationID != "conv_pending_1" || pending[1].ConversationID != "conv_pending_2" {
		t.Errorf("order = %q then %q, want oldest first", pending[0].ConversationID, pending[1].ConversationID)
	}
	if pending[0].CustomerName != "Ana Bertrand" {
		t.Errorf("known caller name = %q, want %q", pending[0].CustomerName, "Ana Bertrand")
	}
	// The unknown caller keeps an empty name and its number: nothing invented.
	if pending[1].CustomerName != "" {
		t.Errorf("unknown caller name = %q, want empty", pending[1].CustomerName)
	}
	if pending[1].Phone != "+596696200002" {
		t.Errorf("unknown caller phone = %q", pending[1].Phone)
	}

	// The other tenant sees only its own.
	otherPending, err := followup.NewReadService(store).Pending(ctxB)
	if err != nil {
		t.Fatalf("Pending() for tenant B error = %v", err)
	}
	if len(otherPending) != 1 || otherPending[0].ConversationID != "conv_pending_3" {
		t.Fatalf("tenant B sees %d requests, want only its own", len(otherPending))
	}
}
