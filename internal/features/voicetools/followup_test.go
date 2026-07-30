package voicetools

import (
	"context"
	"errors"
	"github.com/esrid/garage/internal/adapters/voice"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/core/followup"
)

const voiceFollowUpID = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1"

type followUpStoreStub struct {
	create func(context.Context, string, followup.CreateInput, string) (followup.Request, error)
	called bool
}

func (s *followUpStoreStub) CreateFollowUpRequest(ctx context.Context, tenantID string, input followup.CreateInput, hash string) (followup.Request, error) {
	s.called = true
	return s.create(ctx, tenantID, input, hash)
}

func newFollowUpTool(t *testing.T, store *followUpStoreStub) *FollowUpTool {
	t.Helper()
	authenticator, err := voice.NewTokenAuthenticator(voiceTenantA + ":" + voiceTokenA + "," + voiceTenantB + ":" + voiceTokenB)
	if err != nil {
		t.Fatalf("voice.NewTokenAuthenticator() error = %v", err)
	}
	return NewFollowUpTool(followup.NewService(store), authenticator)
}

func newFollowUpRequest(body, token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/voice/tools/follow-up-request", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func TestFollowUpToolRecordsMinimalTenantScopedResult(t *testing.T) {
	store := &followUpStoreStub{create: func(_ context.Context, tenantID string, input followup.CreateInput, hash string) (followup.Request, error) {
		if tenantID != voiceTenantA || input.ConversationID != "conv_123" || input.Kind != followup.KindCallback || input.Phone != "+596696123456" || input.Details != "Rappeler pour un devis." || len(hash) != 64 {
			t.Fatalf("tenant=%q input=%#v hash=%q", tenantID, input, hash)
		}
		return followup.Request{
			ID: voiceFollowUpID, TenantID: tenantID, CustomerID: voiceCustomerA,
			Kind: input.Kind, Phone: input.Phone, Details: input.Details,
			Status: followup.StatusPending, ConversationID: input.ConversationID,
		}, nil
	}}
	response := httptest.NewRecorder()
	newFollowUpTool(t, store).ServeHTTP(response, newFollowUpRequest(
		`{"conversation_id":"conv_123","kind":"callback","phone":"+596 696 12 34 56","details":" Rappeler pour un devis. "}`,
		voiceTokenA,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	want := `{"recorded":true,"request":{"id":"` + voiceFollowUpID + `","kind":"callback","status":"pending"}}`
	if strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("body=%q, want %q", response.Body.String(), want)
	}
	for _, secret := range []string{voiceTenantA, voiceCustomerA, "+596696123456", "Rappeler", "conv_123", voiceTokenA} {
		if strings.Contains(response.Body.String(), secret) {
			t.Errorf("response leaked %q: %s", secret, response.Body.String())
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("follow-up response is cacheable")
	}
}

func TestFollowUpToolRejectsUnauthorizedAndInvalidRequests(t *testing.T) {
	store := &followUpStoreStub{create: func(context.Context, string, followup.CreateInput, string) (followup.Request, error) {
		t.Fatal("store must not be called")
		return followup.Request{}, nil
	}}
	handler := newFollowUpTool(t, store)
	valid := `{"conversation_id":"conv_123","kind":"callback","phone":"+596696123456","details":"Rappeler."}`
	tests := []struct {
		name        string
		body        string
		token       string
		contentType string
		want        int
	}{
		{"missing auth", valid, "", "application/json", http.StatusUnauthorized},
		{"wrong auth", valid, voiceTokenB + "x", "application/json", http.StatusUnauthorized},
		{"content type", valid, voiceTokenA, "text/plain", http.StatusUnprocessableEntity},
		{"malformed", `{`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"multiple values", valid + `{}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"tenant field", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":"Rappeler","tenant_id":"` + voiceTenantB + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"customer field", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":"Rappeler","customer_id":"` + voiceCustomerA + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"status field", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":"Rappeler","status":"completed"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"idempotency field", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":"Rappeler","idempotency_key":"chosen"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"kind", `{"conversation_id":"conv","kind":"diagnosis","phone":"+596696123456","details":"Rappeler"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"phone", `{"conversation_id":"conv","kind":"callback","phone":"0696123456","details":"Rappeler"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"details", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":""}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"oversized", `{"conversation_id":"conv","kind":"callback","phone":"+596696123456","details":"` + strings.Repeat("x", maxFollowUpBodyBytes) + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newFollowUpRequest(test.body, test.token)
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want || !strings.Contains(response.Body.String(), `"recorded":false`) {
				t.Fatalf("status=%d body=%q, want %d and recorded=false", response.Code, response.Body.String(), test.want)
			}
			if test.want == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("401 response is missing Bearer challenge")
			}
			for _, secret := range []string{voiceTenantA, voiceTokenA} {
				if strings.Contains(response.Body.String(), secret) {
					t.Fatalf("response leaked %q", secret)
				}
			}
		})
	}
	if store.called {
		t.Fatal("invalid request reached store")
	}
}

func TestFollowUpToolMapsFailuresWithoutRecording(t *testing.T) {
	tests := []struct {
		name   string
		result followup.Request
		err    error
		want   int
	}{
		{"conflict", followup.Request{}, followup.ErrIdempotencyConflict, http.StatusConflict},
		{"provider", followup.Request{}, errors.New("postgres secret"), http.StatusServiceUnavailable},
		{"cross tenant", followup.Request{ID: voiceFollowUpID, TenantID: voiceTenantB, Kind: followup.KindCallback, Status: followup.StatusPending}, nil, http.StatusServiceUnavailable},
		{"invalid status", followup.Request{ID: voiceFollowUpID, TenantID: voiceTenantA, Kind: followup.KindCallback, Status: followup.StatusCompleted}, nil, http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &followUpStoreStub{create: func(context.Context, string, followup.CreateInput, string) (followup.Request, error) {
				return test.result, test.err
			}}
			response := httptest.NewRecorder()
			newFollowUpTool(t, store).ServeHTTP(response, newFollowUpRequest(
				`{"conversation_id":"conv_123","kind":"callback","phone":"+596696123456","details":"Rappeler."}`,
				voiceTokenA,
			))
			if response.Code != test.want || !strings.Contains(response.Body.String(), `"recorded":false`) {
				t.Fatalf("status=%d body=%q, want %d and recorded=false", response.Code, response.Body.String(), test.want)
			}
			if strings.Contains(response.Body.String(), "postgres") || strings.Contains(response.Body.String(), voiceTenantB) {
				t.Fatalf("response leaked internals: %q", response.Body.String())
			}
		})
	}
}
