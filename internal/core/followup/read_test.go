package followup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const readTenant = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1"

type pendingStoreStub struct {
	requests []Pending
	err      error
	limit    int
	tenantID string
}

func (s *pendingStoreStub) PendingFollowUps(_ context.Context, tenantID string, limit int) ([]Pending, error) {
	s.tenantID, s.limit = tenantID, limit
	return s.requests, s.err
}

func pendingRequest(id string, createdAt time.Time) Pending {
	return Pending{Request: Request{
		ID: id, TenantID: readTenant, Kind: KindCallback, Phone: "+596696000001",
		Details: "Rappeler pour le devis", Status: StatusPending,
		ConversationID: "conv_" + id, CreatedAt: createdAt,
	}}
}

func TestPendingResolvesTheTenantFromContext(t *testing.T) {
	store := &pendingStoreStub{requests: []Pending{pendingRequest("a", time.Now())}}
	ctx := tenant.WithID(context.Background(), readTenant)

	if _, err := NewReadService(store).Pending(ctx); err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if store.tenantID != readTenant {
		t.Errorf("store received tenant %q, want %q", store.tenantID, readTenant)
	}
	if store.limit != maxPendingFollowUps {
		t.Errorf("store received limit %d, want %d", store.limit, maxPendingFollowUps)
	}
}

// Without a tenant in context there is nothing to read, and the store must not be
// asked: an unauthenticated read is refused before it reaches the database.
func TestPendingRefusesAnAnonymousContext(t *testing.T) {
	store := &pendingStoreStub{}
	_, err := NewReadService(store).Pending(context.Background())

	var unauthorized *domain.UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("error = %v, want UnauthorizedError", err)
	}
	if store.tenantID != "" {
		t.Error("the store was queried without a tenant")
	}
}

// A store answering with another tenant's row, or with something that is not
// pending, is a bug we refuse to render rather than one we pass to the view.
func TestPendingRejectsRowsThatDoNotBelongInTheList(t *testing.T) {
	foreign := pendingRequest("a", time.Now())
	foreign.TenantID = "019c09ea-bca7-7a5d-98b6-000000000000"
	completed := pendingRequest("b", time.Now())
	completed.Status = StatusCompleted
	undated := pendingRequest("c", time.Time{})

	for name, request := range map[string]Pending{
		"foreign tenant": foreign,
		"not pending":    completed,
		"no created_at":  undated,
	} {
		t.Run(name, func(t *testing.T) {
			store := &pendingStoreStub{requests: []Pending{request}}
			if _, err := NewReadService(store).Pending(tenant.WithID(context.Background(), readTenant)); err == nil {
				t.Error("the service returned a row it should have refused")
			}
		})
	}
}

func TestPendingAgeIsRoundedToTheSecond(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	request := pendingRequest("a", now.Add(-90*time.Second-400*time.Millisecond))

	if got := request.Age(now); got != 90*time.Second {
		t.Errorf("Age() = %v, want 1m30s", got)
	}
}
