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
	settings Settings
	written  Settings
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

func (s *storeStub) TenantSettings(context.Context, string) (Settings, error) {
	return s.settings, s.err
}

func (s *storeStub) UpdateTenantSettings(_ context.Context, _ string, settings Settings) (Settings, error) {
	s.written = settings
	return settings, s.err
}

// The transfer number is normalised like every other phone, and clearing it is
// allowed: a workshop with nobody to transfer to should be able to say so rather
// than leave a wrong number in place.
func TestUpdateSettingsNormalisesAndBounds(t *testing.T) {
	ctx := WithID(context.Background(), "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1")

	store := &storeStub{}
	if _, err := NewService(store).UpdateSettings(ctx, Settings{TransferPhone: "+596 696 55 44 33", MonthlyMinutesQuota: 750}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if store.written.TransferPhone != "+596696554433" {
		t.Errorf("transfer phone stored as %q, want it normalised to E.164", store.written.TransferPhone)
	}

	cleared := &storeStub{}
	if _, err := NewService(cleared).UpdateSettings(ctx, Settings{TransferPhone: "  ", MonthlyMinutesQuota: 750}); err != nil {
		t.Fatalf("clearing the number: %v", err)
	}
	if cleared.written.TransferPhone != "" {
		t.Errorf("clearing left %q", cleared.written.TransferPhone)
	}

	for name, settings := range map[string]Settings{
		"unreachable number": {TransferPhone: "allo", MonthlyMinutesQuota: 750},
		"no quota":           {MonthlyMinutesQuota: 0},
		"absurd quota":       {MonthlyMinutesQuota: 1_000_000},
	} {
		t.Run(name, func(t *testing.T) {
			refused := &storeStub{}
			if _, err := NewService(refused).UpdateSettings(ctx, settings); err == nil {
				t.Error("an unusable setting was accepted")
			}
			if refused.written.MonthlyMinutesQuota != 0 {
				t.Error("an unusable setting reached the store")
			}
		})
	}
}
