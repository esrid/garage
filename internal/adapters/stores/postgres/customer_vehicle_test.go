package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
	"github.com/esrid/garage/internal/core/vehicle"
)

func TestCustomerVehicleTenantIsolation(t *testing.T) {
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
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage isolation A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage isolation B"})
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
	customerService := customer.NewService(store)

	privateCustomer, err := customerService.Create(ctxA, customer.CreateInput{
		FirstName: "Ana",
		Phone:     "+596 696 00 00 01",
	})
	if err != nil {
		t.Fatalf("create private customer: %v", err)
	}
	foundPrivate, err := customerService.FindByPhone(ctxA, "+596696000001")
	if err != nil || foundPrivate.ID != privateCustomer.ID || foundPrivate.TenantID != tenantA.ID {
		t.Fatalf("find private customer = %#v, %v", foundPrivate, err)
	}
	_, err = customerService.FindByPhone(ctxB, "+596696000001")
	var notFoundErr *domain.NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("tenant B lookup error = %v, want NotFoundError", err)
	}

	sharedA, err := customerService.Create(ctxA, customer.CreateInput{Phone: "+596696000002"})
	if err != nil {
		t.Fatalf("create shared-phone customer A: %v", err)
	}
	sharedB, err := customerService.Create(ctxB, customer.CreateInput{Phone: "+596696000002"})
	if err != nil {
		t.Fatalf("create shared-phone customer B: %v", err)
	}
	if sharedA.TenantID == sharedB.TenantID {
		t.Fatalf("shared phone resolved to one tenant: A=%#v B=%#v", sharedA, sharedB)
	}
	_, err = customerService.Create(ctxA, customer.CreateInput{Phone: "+596696000002"})
	var duplicateErr *domain.AlreadyExistsError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("duplicate customer error = %v, want AlreadyExistsError", err)
	}

	vehicleService := vehicle.NewService(store)
	vehicleA, err := vehicleService.Create(ctxA, vehicle.CreateInput{
		CustomerID: privateCustomer.ID,
		Plate:      "AB-123-CD",
		Make:       "Renault",
		Model:      "Clio IV",
	})
	if err != nil {
		t.Fatalf("create vehicle A: %v", err)
	}
	_, err = vehicleService.Create(ctxA, vehicle.CreateInput{
		CustomerID: sharedA.ID,
		Plate:      "AB 123 CD",
	})
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("duplicate vehicle error = %v, want AlreadyExistsError", err)
	}
	_, err = vehicleService.Create(ctxB, vehicle.CreateInput{
		CustomerID: privateCustomer.ID,
		Plate:      "ZZ-999-ZZ",
	})
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("cross-tenant vehicle error = %v, want NotFoundError", err)
	}
	_, err = vehicleService.ListByCustomer(ctxB, privateCustomer.ID)
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("cross-tenant vehicle list error = %v, want NotFoundError", err)
	}

	vehicleB, err := vehicleService.Create(ctxB, vehicle.CreateInput{
		CustomerID: sharedB.ID,
		Plate:      "AB-123-CD",
	})
	if err != nil {
		t.Fatalf("create same-plate vehicle B: %v", err)
	}
	foundA, err := vehicleService.FindByPlate(ctxA, "ab 123 cd")
	if err != nil || foundA.ID != vehicleA.ID || foundA.TenantID != tenantA.ID {
		t.Fatalf("find vehicle A = %#v, %v", foundA, err)
	}
	foundB, err := vehicleService.FindByPlate(ctxB, "AB-123-CD")
	if err != nil || foundB.ID != vehicleB.ID || foundB.TenantID != tenantB.ID {
		t.Fatalf("find vehicle B = %#v, %v", foundB, err)
	}

	listed, err := vehicleService.ListByCustomer(ctxA, privateCustomer.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != vehicleA.ID {
		t.Fatalf("list vehicles = %#v, %v", listed, err)
	}

	for _, model := range []string{"Sans plaque 1", "Sans plaque 2"} {
		created, createErr := vehicleService.Create(ctxA, vehicle.CreateInput{
			CustomerID: sharedA.ID,
			Model:      model,
		})
		if createErr != nil || created.NormalizedPlate != "" {
			t.Fatalf("create vehicle without plate = %#v, %v", created, createErr)
		}
	}
	withoutPlates, err := vehicleService.ListByCustomer(ctxA, sharedA.ID)
	if err != nil || len(withoutPlates) != 2 {
		t.Fatalf("vehicles without plates = %#v, %v", withoutPlates, err)
	}
}
