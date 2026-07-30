package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

func TestAuthenticationSessionsAndTenantIsolation(t *testing.T) {
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
	tenantA, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage auth A"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := tenantService.Create(ctx, tenant.CreateInput{Name: "Garage auth B"})
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

	service := coreauth.NewService(store)
	staffA, err := service.Provision(tenant.WithID(ctx, tenantA.ID), coreauth.ProvisionInput{
		Email: " AUTH-A@Example.com ", Password: "correct horse battery staple", Role: coreauth.RoleOwner,
	})
	if err != nil {
		t.Fatalf("provision staff A: %v", err)
	}
	staffB, err := service.Provision(tenant.WithID(ctx, tenantB.ID), coreauth.ProvisionInput{
		Email: "auth-b@example.com", Password: "another sufficiently long password", Role: coreauth.RoleStaff,
	})
	if err != nil {
		t.Fatalf("provision staff B: %v", err)
	}
	if staffA.TenantID != tenantA.ID || staffB.TenantID != tenantB.ID {
		t.Fatalf("provisioned staff A=%#v B=%#v", staffA, staffB)
	}
	var persistedHash string
	if err := store.pool.QueryRow(ctx, `SELECT password_hash FROM staff_users WHERE id = $1::uuid`, staffA.ID).Scan(&persistedHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	if persistedHash == "" || persistedHash == "correct horse battery staple" {
		t.Fatal("raw password was persisted")
	}

	_, err = service.Provision(tenant.WithID(ctx, tenantB.ID), coreauth.ProvisionInput{
		Email: "auth-a@example.com", Password: "another sufficiently long password",
	})
	var duplicate *domain.AlreadyExistsError
	if !errors.As(err, &duplicate) {
		t.Fatalf("duplicate global email error = %v, want AlreadyExistsError", err)
	}

	sessionA, err := service.Login(ctx, "auth-a@example.com", "correct horse battery staple")
	if err != nil {
		t.Fatalf("login A: %v", err)
	}
	sessionB, err := service.Login(ctx, "auth-b@example.com", "another sufficiently long password")
	if err != nil {
		t.Fatalf("login B: %v", err)
	}
	identityA, err := service.Resume(ctx, sessionA.Token)
	if err != nil || identityA.TenantID != tenantA.ID || identityA.StaffID != staffA.ID {
		t.Fatalf("resume A = %#v, %v", identityA, err)
	}
	identityB, err := service.Resume(ctx, sessionB.Token)
	if err != nil || identityB.TenantID != tenantB.ID || identityB.StaffID != staffB.ID {
		t.Fatalf("resume B = %#v, %v", identityB, err)
	}

	wrongTenantIdentity := identityA
	wrongTenantIdentity.TenantID = tenantB.ID
	err = store.CreateBrowserSession(ctx, make([]byte, 32), wrongTenantIdentity, time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("cross-tenant session relation was accepted")
	}

	if _, err := store.pool.Exec(ctx, `UPDATE browser_sessions SET expires_at = now() - interval '1 second' WHERE staff_user_id = $1::uuid`, staffA.ID); err != nil {
		t.Fatalf("expire session A: %v", err)
	}
	if _, err := service.Resume(ctx, sessionA.Token); err == nil {
		t.Fatal("expired session A resumed")
	}
	if _, err := service.Resume(ctx, sessionB.Token); err != nil {
		t.Fatalf("expiring tenant A affected tenant B: %v", err)
	}
	if err := service.Logout(ctx, sessionB.Token); err != nil {
		t.Fatalf("logout B: %v", err)
	}
	if _, err := service.Resume(ctx, sessionB.Token); err == nil {
		t.Fatal("revoked session B resumed")
	}
}

func TestAuthenticationPersistsAccountLock(t *testing.T) {
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
	tenantValue, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: "Garage auth lock"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := store.pool.Exec(context.Background(), "DELETE FROM tenants WHERE id = $1::uuid", tenantValue.ID); cleanupErr != nil {
			t.Errorf("cleanup tenant: %v", cleanupErr)
		}
	})
	service := coreauth.NewService(store)
	staff, err := service.Provision(tenant.WithID(ctx, tenantValue.ID), coreauth.ProvisionInput{
		Email: "locked@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		_, loginErr := service.Login(ctx, staff.Email, "wrong password")
		var unauthorized *domain.UnauthorizedError
		if !errors.As(loginErr, &unauthorized) {
			t.Fatalf("wrong password attempt %d error = %v, want UnauthorizedError", attempt+1, loginErr)
		}
	}
	var failures int
	var lockedUntil *time.Time
	if err := store.pool.QueryRow(ctx, `SELECT failed_login_attempts, locked_until FROM staff_users WHERE id = $1::uuid`, staff.ID).Scan(&failures, &lockedUntil); err != nil {
		t.Fatalf("read lock state: %v", err)
	}
	if failures != 5 || lockedUntil == nil || !lockedUntil.After(time.Now()) {
		t.Fatalf("lock state failures=%d lockedUntil=%v", failures, lockedUntil)
	}
	if _, err := service.Login(ctx, staff.Email, "correct horse battery staple"); err == nil {
		t.Fatal("persistently locked account logged in")
	}
}
