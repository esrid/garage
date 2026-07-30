package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type storeStub struct {
	staff         Staff
	createInput   ProvisionInput
	createdTenant string
	createdHash   string
	sessionHash   []byte
	session       Identity
	expiresAt     time.Time
	deletedHash   []byte
	failures      int
	lockedUntil   *time.Time
	err           error
}

func (s *storeStub) CreateStaff(_ context.Context, tenantID string, record CreateStaffRecord) (Staff, error) {
	s.createdTenant = tenantID
	s.createInput = ProvisionInput{Email: record.Email, DisplayName: record.DisplayName, Role: record.Role}
	s.createdHash = record.PasswordHash
	if s.err != nil {
		return Staff{}, s.err
	}
	s.staff = Staff{ID: "staff-1", TenantID: tenantID, Email: record.Email, DisplayName: record.DisplayName, Role: record.Role}
	return s.staff, nil
}

func (s *storeStub) FindStaffByEmail(_ context.Context, email string) (Credentials, error) {
	if s.err != nil {
		return Credentials{}, s.err
	}
	if s.staff.Email != email {
		return Credentials{}, &domain.NotFoundError{Entity: "staff"}
	}
	return Credentials{
		Staff: s.staff, PasswordHash: s.createdHash,
		FailedLoginAttempts: s.failures, LockedUntil: s.lockedUntil,
	}, nil
}

func (s *storeStub) RecordLoginFailure(_ context.Context, _ string, now, lockUntil time.Time, limit int) error {
	if s.err != nil {
		return s.err
	}
	if s.lockedUntil != nil && !s.lockedUntil.After(now) {
		s.failures = 0
	}
	s.failures++
	if s.failures >= limit {
		s.lockedUntil = &lockUntil
	}
	return nil
}

func (s *storeStub) ClearLoginFailures(context.Context, string) error {
	s.failures, s.lockedUntil = 0, nil
	return s.err
}

func (s *storeStub) CreateBrowserSession(_ context.Context, hash []byte, identity Identity, expiresAt time.Time) error {
	s.sessionHash = append([]byte(nil), hash...)
	s.session, s.expiresAt = identity, expiresAt
	return s.err
}

func (s *storeStub) FindBrowserSession(_ context.Context, hash []byte, now time.Time) (Identity, error) {
	if s.err != nil {
		return Identity{}, s.err
	}
	if string(hash) != string(s.sessionHash) || !s.expiresAt.After(now) {
		return Identity{}, &domain.NotFoundError{Entity: "session"}
	}
	return s.session, nil
}

func (s *storeStub) DeleteBrowserSession(_ context.Context, hash []byte) error {
	s.deletedHash = append([]byte(nil), hash...)
	if string(hash) == string(s.sessionHash) {
		s.sessionHash = nil
	}
	return s.err
}

func TestPasswordHashRoundTripAndUniqueSalt(t *testing.T) {
	first, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	second, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("second hashPassword() error = %v", err)
	}
	if first == second {
		t.Fatal("password hashes reuse a salt")
	}
	matched, err := verifyPassword("correct horse battery staple", first)
	if err != nil || !matched {
		t.Fatalf("verify correct password = %v, %v", matched, err)
	}
	matched, err = verifyPassword("wrong password", first)
	if err != nil || matched {
		t.Fatalf("verify wrong password = %v, %v", matched, err)
	}
}

func TestProvisionUsesTenantContextAndNormalizesEmail(t *testing.T) {
	store := &storeStub{}
	service := NewService(store)
	ctx := tenant.WithID(context.Background(), "tenant-from-context")
	created, err := service.Provision(ctx, ProvisionInput{
		Email:       "  GARAGE@Example.COM ",
		DisplayName: "  Ana  ",
		Password:    "a sufficiently long password",
		Role:        RoleOwner,
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if store.createdTenant != "tenant-from-context" || store.createInput.Email != "garage@example.com" || created.TenantID != "tenant-from-context" {
		t.Fatalf("provision tenant=%q input=%#v created=%#v", store.createdTenant, store.createInput, created)
	}
	if store.createdHash == "" || store.createdHash == "a sufficiently long password" {
		t.Fatal("raw password was stored")
	}

	_, err = service.Provision(context.Background(), ProvisionInput{Email: "other@example.com", Password: "a sufficiently long password"})
	var unauthorized *domain.UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("missing tenant error = %v, want UnauthorizedError", err)
	}
}

func TestLoginResumeLogoutAndLockout(t *testing.T) {
	passwordHash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	store := &storeStub{staff: Staff{
		ID: "staff-1", TenantID: "tenant-1", Email: "garage@example.com",
		DisplayName: "Ana", Role: RoleOwner,
	}, createdHash: passwordHash}
	service := NewService(store)
	service.now = func() time.Time { return now }

	for attempt := 1; attempt <= loginFailureLimit; attempt++ {
		_, err := service.Login(context.Background(), "garage@example.com", "wrong password")
		var unauthorized *domain.UnauthorizedError
		if !errors.As(err, &unauthorized) {
			t.Fatalf("attempt %d error = %v, want UnauthorizedError", attempt, err)
		}
	}
	if store.lockedUntil == nil || !store.lockedUntil.Equal(now.Add(loginLockDuration)) {
		t.Fatalf("lockedUntil = %v", store.lockedUntil)
	}
	if _, err := service.Login(context.Background(), "garage@example.com", "correct horse battery staple"); err == nil {
		t.Fatal("locked account logged in")
	}

	now = now.Add(loginLockDuration + time.Second)
	session, err := service.Login(context.Background(), " GARAGE@example.com ", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if len(session.Token) != 43 || session.Identity.TenantID != "tenant-1" || !session.ExpiresAt.Equal(now.Add(SessionLifetime)) {
		t.Fatalf("Login() session = %#v", session)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(session.Token)
	if err != nil {
		t.Fatalf("decode session token: %v", err)
	}
	wantHash := sha256.Sum256(rawToken)
	if string(store.sessionHash) != string(wantHash[:]) || string(store.sessionHash) == string(rawToken) {
		t.Fatal("session store did not receive only the token digest")
	}
	identity, err := service.Resume(context.Background(), session.Token)
	if err != nil || identity.TenantID != "tenant-1" || identity.StaffID != "staff-1" {
		t.Fatalf("Resume() = %#v, %v", identity, err)
	}
	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Resume(context.Background(), session.Token); err == nil {
		t.Fatal("revoked session resumed")
	}
}

func TestUnknownAndMalformedCredentialsAreUnauthorized(t *testing.T) {
	service := NewService(&storeStub{})
	for _, test := range []struct{ email, password string }{
		{"unknown@example.com", "wrong password"},
		{"not-an-email", "wrong password"},
		{"unknown@example.com", string(make([]byte, MaxPasswordBytes+1))},
	} {
		_, err := service.Login(context.Background(), test.email, test.password)
		var unauthorized *domain.UnauthorizedError
		if !errors.As(err, &unauthorized) {
			t.Fatalf("Login(%q) error = %v, want UnauthorizedError", test.email, err)
		}
	}
	if _, err := service.Resume(context.Background(), "malformed"); err == nil {
		t.Fatal("malformed session resumed")
	}
}
