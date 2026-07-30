package vehicle

import (
	"context"
	"errors"
	"testing"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const customerID = "01980000-0000-7000-8000-000000000001"

type storeStub struct {
	tenantID        string
	input           CreateInput
	normalizedPlate string
	customerID      string
	called          bool
}

func (s *storeStub) CreateVehicle(_ context.Context, tenantID string, input CreateInput, normalizedPlate string) (Vehicle, error) {
	s.called = true
	s.tenantID = tenantID
	s.input = input
	s.normalizedPlate = normalizedPlate
	return Vehicle{ID: "vehicle-1"}, nil
}

func (s *storeStub) FindVehicleByPlate(_ context.Context, tenantID, normalizedPlate string) (Vehicle, error) {
	s.called = true
	s.tenantID = tenantID
	s.normalizedPlate = normalizedPlate
	return Vehicle{ID: "vehicle-1"}, nil
}

func (s *storeStub) ListVehiclesByCustomer(_ context.Context, tenantID, customerID string) ([]Vehicle, error) {
	s.called = true
	s.tenantID = tenantID
	s.customerID = customerID
	return []Vehicle{{ID: "vehicle-1"}}, nil
}

func TestNormalizePlate(t *testing.T) {
	got, err := NormalizePlate(" ab-123-cd ")
	if err != nil || got != "AB123CD" {
		t.Fatalf("NormalizePlate() = %q, %v", got, err)
	}
}

func TestNormalizePlateRejectsInvalidValue(t *testing.T) {
	for _, input := range []string{"", "A", "AB_123", "éé-123"} {
		_, err := NormalizePlate(input)
		var validationErr *domain.ValidationError
		if !errors.As(err, &validationErr) {
			t.Errorf("NormalizePlate(%q) error = %v, want ValidationError", input, err)
		}
	}
}

func TestServiceUsesTenantFromContext(t *testing.T) {
	store := &storeStub{}
	service := NewService(store)
	ctx := tenant.WithID(context.Background(), "tenant-1")

	_, err := service.Create(ctx, CreateInput{CustomerID: customerID, Plate: " ab-123-cd ", Make: " Renault "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if store.tenantID != "tenant-1" || store.normalizedPlate != "AB123CD" || store.input.Make != "Renault" {
		t.Fatalf("store tenant=%q normalizedPlate=%q input=%#v", store.tenantID, store.normalizedPlate, store.input)
	}

	_, err = service.ListByCustomer(ctx, customerID)
	if err != nil || store.tenantID != "tenant-1" || store.customerID != customerID {
		t.Fatalf("ListByCustomer() error=%v tenant=%q customer=%q", err, store.tenantID, store.customerID)
	}
}

func TestServiceRejectsMissingTenantBeforeStore(t *testing.T) {
	store := &storeStub{}
	service := NewService(store)
	_, err := service.FindByPlate(context.Background(), "AB-123-CD")
	var unauthorizedErr *domain.UnauthorizedError
	if !errors.As(err, &unauthorizedErr) || store.called {
		t.Fatalf("FindByPlate() error=%v called=%t", err, store.called)
	}
}
