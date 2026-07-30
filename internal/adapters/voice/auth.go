package voice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	minToolTokenLength = 32
	maxToolTokenLength = 200
)

type TokenAuthenticator struct {
	tenantByTokenHash map[[sha256.Size]byte]string
}

func NewTokenAuthenticator(encoded string) (*TokenAuthenticator, error) {
	authenticator := &TokenAuthenticator{tenantByTokenHash: make(map[[sha256.Size]byte]string)}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return authenticator, nil
	}
	seenTenants := make(map[string]struct{})
	for _, item := range strings.Split(encoded, ",") {
		if strings.Count(item, ":") != 1 {
			return nil, fmt.Errorf("voice credentials: each entry must be tenant-uuid:token")
		}
		tenantID, token, _ := strings.Cut(item, ":")
		tenantID = strings.ToLower(strings.TrimSpace(tenantID))
		if !validUUID(tenantID) {
			return nil, fmt.Errorf("voice credentials: tenant ID must be a UUID")
		}
		if !validToolToken(token) {
			return nil, fmt.Errorf("voice credentials: token must contain 32 to 200 visible characters without comma or colon")
		}
		if _, exists := seenTenants[tenantID]; exists {
			return nil, fmt.Errorf("voice credentials: duplicate tenant")
		}
		hash := sha256.Sum256([]byte(token))
		if _, exists := authenticator.tenantByTokenHash[hash]; exists {
			return nil, fmt.Errorf("voice credentials: token must be unique per tenant")
		}
		seenTenants[tenantID] = struct{}{}
		authenticator.tenantByTokenHash[hash] = tenantID
	}
	return authenticator, nil
}

func (a *TokenAuthenticator) Authenticate(ctx context.Context, authorization string) (context.Context, error) {
	scheme, token, found := strings.Cut(strings.TrimSpace(authorization), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !validToolToken(token) {
		return ctx, &domain.UnauthorizedError{Message: "voice tool authentication required"}
	}
	tenantID, ok := a.tenantByTokenHash[sha256.Sum256([]byte(token))]
	if !ok {
		return ctx, &domain.UnauthorizedError{Message: "voice tool authentication required"}
	}
	return tenant.WithID(ctx, tenantID), nil
}

func validToolToken(token string) bool {
	if len(token) < minToolTokenLength || len(token) > maxToolTokenLength || strings.ContainsAny(token, ",:") {
		return false
	}
	for _, character := range []byte(token) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}
