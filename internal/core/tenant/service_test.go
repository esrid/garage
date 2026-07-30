package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
)

type storeStub struct {
	name     string
	timezone string
	result   Tenant
	err      error
}

func (s *storeStub) CreateTenant(_ context.Context, name, timezone string) (Tenant, error) {
	s.name = name
	s.timezone = timezone
	return s.result, s.err
}

func TestServiceCreateDefaultsTimezoneAndTrimsName(t *testing.T) {
	store := &storeStub{result: Tenant{ID: "tenant-1"}}
	service := NewService(store)

	created, err := service.Create(context.Background(), CreateInput{Name: "  Garage Madinina  "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != "tenant-1" || store.name != "Garage Madinina" || store.timezone != DefaultTimezone {
		t.Fatalf("Create() = %#v, store name=%q timezone=%q", created, store.name, store.timezone)
	}
}

func TestServiceCreateValidatesInput(t *testing.T) {
	service := NewService(&storeStub{})
	_, err := service.Create(context.Background(), CreateInput{Timezone: "Mars/Olympus"})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create() error = %v, want ValidationError", err)
	}
	if validationErr.Errors["name"] == "" || validationErr.Errors["timezone"] == "" {
		t.Fatalf("validation errors = %#v", validationErr.Errors)
	}
}

func TestTenantContext(t *testing.T) {
	ctx := WithID(context.Background(), " tenant-1 ")
	tenantID, err := IDFromContext(ctx)
	if err != nil || tenantID != "tenant-1" {
		t.Fatalf("IDFromContext() = %q, %v", tenantID, err)
	}

	_, err = IDFromContext(context.Background())
	var unauthorizedErr *domain.UnauthorizedError
	if !errors.As(err, &unauthorizedErr) {
		t.Fatalf("IDFromContext() error = %v, want UnauthorizedError", err)
	}
}

func TestServiceCreatePropagatesStoreError(t *testing.T) {
	wantErr := errors.New("database down")
	service := NewService(&storeStub{err: wantErr})
	_, err := service.Create(context.Background(), CreateInput{Name: "Garage", Timezone: time.UTC.String()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Create() error = %v, want %v", err, wantErr)
	}
}
