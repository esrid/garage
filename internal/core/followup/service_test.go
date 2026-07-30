package followup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type storeStub struct {
	tenantID    string
	input       CreateInput
	requestHash string
	result      Request
	err         error
	called      bool
}

func (s *storeStub) CreateFollowUpRequest(_ context.Context, tenantID string, input CreateInput, hash string) (Request, error) {
	s.called = true
	s.tenantID = tenantID
	s.input = input
	s.requestHash = hash
	return s.result, s.err
}

func TestServiceCreatesNormalizedTenantRequest(t *testing.T) {
	store := &storeStub{result: Request{ID: "request-1", Status: StatusPending}}
	service := NewService(store)
	ctx := tenant.WithID(context.Background(), "tenant-1")
	created, err := service.Create(ctx, CreateInput{
		ConversationID: " conv_123 ",
		Kind:           KindCallback,
		Phone:          "+596 696-12-34-56",
		Details:        " Besoin d'un rappel. ",
	})
	if err != nil || created.ID != "request-1" {
		t.Fatalf("Create() = %#v, %v", created, err)
	}
	if store.tenantID != "tenant-1" || store.input.ConversationID != "conv_123" || store.input.Phone != "+596696123456" || store.input.Details != "Besoin d'un rappel." {
		t.Fatalf("store tenant=%q input=%#v", store.tenantID, store.input)
	}
	if len(store.requestHash) != 64 || store.requestHash != requestHash(store.input.Phone, store.input.Details) {
		t.Fatalf("request hash = %q", store.requestHash)
	}
}

func TestServiceRejectsMissingTenantBeforeStore(t *testing.T) {
	store := &storeStub{}
	_, err := NewService(store).Create(context.Background(), validInput())
	var unauthorized *domain.UnauthorizedError
	if !errors.As(err, &unauthorized) || store.called {
		t.Fatalf("Create() error=%v called=%t", err, store.called)
	}
}

func TestServiceRejectsInvalidInputBeforeStore(t *testing.T) {
	valid := validInput()
	tests := []struct {
		name  string
		input CreateInput
	}{
		{"conversation missing", CreateInput{Kind: valid.Kind, Phone: valid.Phone, Details: valid.Details}},
		{"conversation too long", CreateInput{ConversationID: strings.Repeat("x", 513), Kind: valid.Kind, Phone: valid.Phone, Details: valid.Details}},
		{"kind", CreateInput{ConversationID: valid.ConversationID, Kind: "diagnosis", Phone: valid.Phone, Details: valid.Details}},
		{"phone", CreateInput{ConversationID: valid.ConversationID, Kind: valid.Kind, Phone: "0696123456", Details: valid.Details}},
		{"details missing", CreateInput{ConversationID: valid.ConversationID, Kind: valid.Kind, Phone: valid.Phone}},
		{"details too long", CreateInput{ConversationID: valid.ConversationID, Kind: valid.Kind, Phone: valid.Phone, Details: strings.Repeat("é", 1001)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &storeStub{}
			_, err := NewService(store).Create(tenant.WithID(context.Background(), "tenant-1"), test.input)
			var validation *domain.ValidationError
			if !errors.As(err, &validation) || store.called {
				t.Fatalf("Create() error=%v called=%t", err, store.called)
			}
		})
	}
}

func TestRequestHashDistinguishesFieldBoundaries(t *testing.T) {
	if requestHash("a", "bc") == requestHash("ab", "c") {
		t.Fatal("different normalized requests share a hash")
	}
}

func validInput() CreateInput {
	return CreateInput{
		ConversationID: "conv_123",
		Kind:           KindCallback,
		Phone:          "+596696123456",
		Details:        "Besoin d'un rappel.",
	}
}
