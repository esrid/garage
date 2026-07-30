package customer

import (
	"context"
	"errors"
	"testing"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type storeStub struct {
	tenantID string
	input    CreateInput
	phone    string
	result   Customer
	called   bool
}

func (s *storeStub) CreateCustomer(_ context.Context, tenantID string, input CreateInput) (Customer, error) {
	s.called = true
	s.tenantID = tenantID
	s.input = input
	return s.result, nil
}

func (s *storeStub) FindCustomerByPhone(_ context.Context, tenantID, phone string) (Customer, error) {
	s.called = true
	s.tenantID = tenantID
	s.phone = phone
	return s.result, nil
}

func TestNormalizePhone(t *testing.T) {
	tests := map[string]string{
		"plus prefix": "+596 696-12-34-56",
		"00 prefix":   "00596 (696) 12.34.56",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizePhone(input)
			if err != nil || got != "+596696123456" {
				t.Fatalf("NormalizePhone(%q) = %q, %v", input, got, err)
			}
		})
	}
}

func TestNormalizePhoneRejectsNationalOrInvalidNumber(t *testing.T) {
	for _, input := range []string{"0696123456", "+596CALLNOW", "+123"} {
		_, err := NormalizePhone(input)
		var validationErr *domain.ValidationError
		if !errors.As(err, &validationErr) {
			t.Errorf("NormalizePhone(%q) error = %v, want ValidationError", input, err)
		}
	}
}

func TestServiceUsesTenantFromContext(t *testing.T) {
	store := &storeStub{result: Customer{ID: "customer-1"}}
	service := NewService(store)
	ctx := tenant.WithID(context.Background(), "tenant-1")

	_, err := service.Create(ctx, CreateInput{FirstName: "  Ana ", Phone: "+596 696 12 34 56"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if store.tenantID != "tenant-1" || store.input.FirstName != "Ana" || store.input.Phone != "+596696123456" {
		t.Fatalf("store tenant=%q input=%#v", store.tenantID, store.input)
	}

	_, err = service.FindByPhone(ctx, "00596 696 12 34 56")
	if err != nil || store.tenantID != "tenant-1" || store.phone != "+596696123456" {
		t.Fatalf("FindByPhone() error=%v tenant=%q phone=%q", err, store.tenantID, store.phone)
	}
}

func TestServiceRejectsMissingTenantBeforeStore(t *testing.T) {
	store := &storeStub{}
	service := NewService(store)
	_, err := service.FindByPhone(context.Background(), "+596696123456")
	var unauthorizedErr *domain.UnauthorizedError
	if !errors.As(err, &unauthorizedErr) || store.called {
		t.Fatalf("FindByPhone() error=%v called=%t", err, store.called)
	}
}
