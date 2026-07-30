package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	RoleOwner          = "owner"
	RoleStaff          = "staff"
	SessionCookieName  = "__Host-garage_session"
	SessionLifetime    = 12 * time.Hour
	MaxPasswordBytes   = 128
	minPasswordBytes   = 12
	maxEmailBytes      = 254
	maxDisplayNameRune = 100
	loginFailureLimit  = 5
	loginLockDuration  = 15 * time.Minute
)

type Staff struct {
	ID          string
	TenantID    string
	Email       string
	DisplayName string
	Role        string
	Disabled    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Identity struct {
	StaffID     string
	TenantID    string
	Email       string
	DisplayName string
	Role        string
}

type Session struct {
	Token     string
	ExpiresAt time.Time
	Identity  Identity
}

type ProvisionInput struct {
	Email       string
	DisplayName string
	Password    string
	Role        string
}

type CreateStaffRecord struct {
	Email        string
	DisplayName  string
	Role         string
	PasswordHash string
}

type Credentials struct {
	Staff               Staff
	PasswordHash        string
	FailedLoginAttempts int
	LockedUntil         *time.Time
}

type Store interface {
	CreateStaff(ctx context.Context, tenantID string, record CreateStaffRecord) (Staff, error)
	FindStaffByEmail(ctx context.Context, email string) (Credentials, error)
	RecordLoginFailure(ctx context.Context, staffID string, now, lockUntil time.Time, limit int) error
	ClearLoginFailures(ctx context.Context, staffID string) error
	CreateBrowserSession(ctx context.Context, tokenHash []byte, identity Identity, expiresAt time.Time) error
	FindBrowserSession(ctx context.Context, tokenHash []byte, now time.Time) (Identity, error)
	DeleteBrowserSession(ctx context.Context, tokenHash []byte) error
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) Provision(ctx context.Context, input ProvisionInput) (Staff, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Staff{}, err
	}

	input.Email = normalizeEmail(input.Email)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.TrimSpace(input.Role)
	if input.Role == "" {
		input.Role = RoleStaff
	}
	validationErrors := make(map[string]string)
	if !validEmail(input.Email) {
		validationErrors["email"] = "must be a valid email address"
	}
	if len([]byte(input.Password)) < minPasswordBytes || len([]byte(input.Password)) > MaxPasswordBytes {
		validationErrors["password"] = "must contain 12 to 128 bytes"
	}
	if len([]rune(input.DisplayName)) > maxDisplayNameRune {
		validationErrors["display_name"] = "must contain at most 100 characters"
	}
	if input.Role != RoleOwner && input.Role != RoleStaff {
		validationErrors["role"] = "must be owner or staff"
	}
	if len(validationErrors) > 0 {
		return Staff{}, &domain.ValidationError{Entity: "staff", Errors: validationErrors}
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return Staff{}, fmt.Errorf("auth: hash password: %w", err)
	}
	return s.store.CreateStaff(ctx, tenantID, CreateStaffRecord{
		Email: input.Email, DisplayName: input.DisplayName, Role: input.Role, PasswordHash: passwordHash,
	})
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	email = normalizeEmail(email)
	lookupAllowed := validEmail(email) && len([]byte(password)) <= MaxPasswordBytes
	var credentials Credentials
	var lookupErr error
	if lookupAllowed {
		credentials, lookupErr = s.store.FindStaffByEmail(ctx, email)
	}

	if !lookupAllowed || isNotFound(lookupErr) {
		if err := burnUnknownPassword(password); err != nil {
			return Session{}, fmt.Errorf("auth: verify credentials: %w", err)
		}
		return Session{}, unauthorized()
	}
	if lookupErr != nil {
		return Session{}, fmt.Errorf("auth: find staff: %w", lookupErr)
	}

	passwordMatches, err := verifyPassword(password, credentials.PasswordHash)
	if err != nil {
		return Session{}, fmt.Errorf("auth: verify password: %w", err)
	}
	now := s.now().UTC()
	locked := credentials.LockedUntil != nil && credentials.LockedUntil.After(now)
	if !passwordMatches {
		if !locked {
			if err := s.store.RecordLoginFailure(ctx, credentials.Staff.ID, now, now.Add(loginLockDuration), loginFailureLimit); err != nil {
				return Session{}, fmt.Errorf("auth: record login failure: %w", err)
			}
		}
		return Session{}, unauthorized()
	}
	if credentials.Staff.Disabled || locked {
		return Session{}, unauthorized()
	}
	if err := s.store.ClearLoginFailures(ctx, credentials.Staff.ID); err != nil {
		return Session{}, fmt.Errorf("auth: clear login failures: %w", err)
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		return Session{}, fmt.Errorf("auth: create session token: %w", err)
	}
	identity := staffIdentity(credentials.Staff)
	expiresAt := now.Add(SessionLifetime)
	if err := s.store.CreateBrowserSession(ctx, tokenHash, identity, expiresAt); err != nil {
		return Session{}, fmt.Errorf("auth: persist session: %w", err)
	}
	return Session{Token: token, ExpiresAt: expiresAt, Identity: identity}, nil
}

func (s *Service) Resume(ctx context.Context, token string) (Identity, error) {
	tokenHash, err := sessionTokenHash(token)
	if err != nil {
		return Identity{}, unauthorized()
	}
	identity, err := s.store.FindBrowserSession(ctx, tokenHash, s.now().UTC())
	if isNotFound(err) {
		return Identity{}, unauthorized()
	}
	if err != nil {
		return Identity{}, fmt.Errorf("auth: find session: %w", err)
	}
	return identity, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	tokenHash, err := sessionTokenHash(token)
	if err != nil {
		return nil
	}
	if err := s.store.DeleteBrowserSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// validEmail parses with net/mail, then applies our own policy on top.
//
// The parser is stdlib because address syntax is not something to re-derive by
// counting "@". The extra rules are deliberate policy, not parsing: mail.
// ParseAddress accepts "Name <a@b>" and a domain with no dot, neither of which
// is a sign-in identifier we want to store.
func validEmail(value string) bool {
	if value == "" || len(value) > maxEmailBytes || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Name != "" || address.Address != value {
		return false
	}
	local, domainName, _ := strings.Cut(address.Address, "@")
	return local != "" && len(local) <= 64 && strings.Contains(domainName, ".") &&
		!strings.HasPrefix(domainName, ".") && !strings.HasSuffix(domainName, ".")
}

func staffIdentity(staff Staff) Identity {
	return Identity{
		StaffID:     staff.ID,
		TenantID:    staff.TenantID,
		Email:       staff.Email,
		DisplayName: staff.DisplayName,
		Role:        staff.Role,
	}
}

func newSessionToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256(raw)
	return token, digest[:], nil
}

func sessionTokenHash(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(raw) != 32 {
		return nil, errors.New("invalid session token")
	}
	digest := sha256.Sum256(raw)
	return digest[:], nil
}

func isNotFound(err error) bool {
	var notFound *domain.NotFoundError
	return errors.As(err, &notFound)
}

func unauthorized() error {
	return &domain.UnauthorizedError{Message: "invalid credentials"}
}

type identityContextKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, error) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	if !ok || identity.StaffID == "" || identity.TenantID == "" {
		return Identity{}, &domain.UnauthorizedError{Message: "staff context required"}
	}
	return identity, nil
}
