package voice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	voiceTenantA = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1"
	voiceTenantB = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea2"
	voiceTokenA  = "voice-tool-token-a-0123456789abcdef"
	voiceTokenB  = "voice-tool-token-b-0123456789abcdef"
)

func TestTokenAuthenticatorResolvesTenantWithoutAcceptingTenantInput(t *testing.T) {
	authenticator, err := NewTokenAuthenticator(voiceTenantA + ":" + voiceTokenA + "," + voiceTenantB + ":" + voiceTokenB)
	if err != nil {
		t.Fatalf("NewTokenAuthenticator() error = %v", err)
	}
	ctx, err := authenticator.Authenticate(context.Background(), "Bearer "+voiceTokenB)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	gotTenant, err := tenant.IDFromContext(ctx)
	if err != nil || gotTenant != voiceTenantB {
		t.Fatalf("tenant = %q, %v", gotTenant, err)
	}
}

func TestTokenAuthenticatorRejectsInvalidCredentials(t *testing.T) {
	authenticator, err := NewTokenAuthenticator(voiceTenantA + ":" + voiceTokenA)
	if err != nil {
		t.Fatalf("NewTokenAuthenticator() error = %v", err)
	}
	for _, authorization := range []string{"", "Basic " + voiceTokenA, "Bearer short", "Bearer " + voiceTokenB} {
		_, err := authenticator.Authenticate(context.Background(), authorization)
		var unauthorized *domain.UnauthorizedError
		if !errors.As(err, &unauthorized) {
			t.Errorf("Authenticate(%q) error = %v, want UnauthorizedError", authorization, err)
		}
	}
}

func TestNewTokenAuthenticatorRejectsUnsafeConfiguration(t *testing.T) {
	tests := []string{
		"not-an-entry",
		"not-a-uuid:" + voiceTokenA,
		voiceTenantA + ":short",
		voiceTenantA + ":" + voiceTokenA + "," + voiceTenantA + ":" + voiceTokenB,
		voiceTenantA + ":" + voiceTokenA + "," + strings.ToUpper(voiceTenantA) + ":" + voiceTokenB,
		voiceTenantA + ":" + voiceTokenA + "," + voiceTenantB + ":" + voiceTokenA,
		voiceTenantA + ":" + strings.Repeat("x", maxToolTokenLength+1),
	}
	for _, encoded := range tests {
		if _, err := NewTokenAuthenticator(encoded); err == nil {
			t.Errorf("NewTokenAuthenticator(%q) succeeded", encoded)
		}
	}
}
