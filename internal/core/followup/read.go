package followup

import (
	"context"
	"fmt"
	"time"

	"github.com/esrid/garage/internal/core/tenant"
)

// maxPendingFollowUps bounds one read. The dashboard shows what the desk can act
// on today; a tenant with more pending requests than this has a backlog problem
// that a longer list would not solve.
const maxPendingFollowUps = 200

// ReadStore is the narrow persistence contract the desk views need. tenantID is
// always resolved by the service, never accepted from HTTP.
type ReadStore interface {
	PendingFollowUps(ctx context.Context, tenantID string, limit int) ([]Pending, error)
	CallersByConversation(ctx context.Context, tenantID string, conversationIDs []string) (map[string]Caller, error)
}

// PendingReader is the read capability consumed by the HTTP adapter.
type PendingReader interface {
	Pending(ctx context.Context) ([]Pending, error)
}

// Pending is a follow-up request waiting to be handled, with the customer name
// resolved when the request is attached to a known customer.
type Pending struct {
	Request
	CustomerName string
}

type ReadService struct {
	store ReadStore
}

func NewReadService(store ReadStore) *ReadService {
	return &ReadService{store: store}
}

// Pending lists the requests still to handle, oldest first: the desk works a
// queue, and the one that has waited longest is the one that matters.
func (s *ReadService) Pending(ctx context.Context) ([]Pending, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	requests, err := s.store.PendingFollowUps(ctx, tenantID, maxPendingFollowUps)
	if err != nil {
		return nil, err
	}
	// A store that answered with another tenant's row is a bug we refuse to
	// render, not one we paper over: the same defence the conversation history
	// applies after its own read.
	for _, request := range requests {
		if request.TenantID != tenantID {
			return nil, fmt.Errorf("follow-up store returned a foreign request")
		}
		if request.Status != StatusPending {
			return nil, fmt.Errorf("follow-up store returned a %q request in the pending list", request.Status)
		}
		if request.CreatedAt.IsZero() {
			return nil, fmt.Errorf("follow-up store returned a request without a creation time")
		}
	}
	return requests, nil
}

// Age is how long a request has been waiting, for a desk that wants to know what
// is going stale. Rounded to the second: nanoseconds are noise here.
func (p Pending) Age(now time.Time) time.Duration {
	return now.Sub(p.CreatedAt).Round(time.Second)
}

// Caller is who a conversation was with, as recorded by our own voice tools
// during the call. It is never derived from the provider payload: the ElevenLabs
// post-call event documents no caller number (checked against
// https://elevenlabs.io/docs/eleven-agents/workflows/post-call-webhooks on
// 2026-07-30), so inventing one from an undocumented field is not an option.
type Caller struct {
	Phone        string
	CustomerName string
}

// CallerDirectory is the read capability consumed by the call history adapter.
type CallerDirectory interface {
	Callers(ctx context.Context, conversationIDs []string) (map[string]Caller, error)
}

// Callers returns the caller of each conversation we hold a follow-up request
// for. Conversations with no request are simply absent from the map: the history
// then shows what it knows, which is the hour and the outcome.
//
// One query for the whole page, not one per row.
func (s *ReadService) Callers(ctx context.Context, conversationIDs []string) (map[string]Caller, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(conversationIDs) == 0 {
		return map[string]Caller{}, nil
	}
	return s.store.CallersByConversation(ctx, tenantID, conversationIDs)
}
