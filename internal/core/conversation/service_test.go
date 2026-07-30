package conversation

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const conversationTenantID = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1"

type conversationStoreStub struct {
	record func(context.Context, string, RecordInput, [sha256.Size]byte) (RecordResult, error)
	called bool
}

func (s *conversationStoreStub) RecordPostCall(ctx context.Context, tenantID string, input RecordInput, hash [sha256.Size]byte) (RecordResult, error) {
	s.called = true
	return s.record(ctx, tenantID, input, hash)
}

func validRecordInput() RecordInput {
	cost := int64(1_100_000)
	return RecordInput{
		AgentID:                "agent_123",
		ProviderConversationID: "conv_123",
		ProviderStatus:         "done",
		ProviderEventAt:        time.Unix(1739537297, 0).UTC(),
		StartedAt:              time.Unix(1739537275, 0).UTC(),
		DurationSeconds:        22,
		CostFiatMicroUSD:       &cost,
		Transcript:             []byte(`[{"role":"user","message":"Bonjour"}]`),
		Summary:                "Demande de rendez-vous.",
		ProviderOutcome:        "success",
		Analysis:               []byte(`{"call_successful":"success"}`),
		Metadata:               []byte(`{"call_duration_secs":22}`),
		RawPayload:             []byte(`{"type":"post_call_transcription"}`),
	}
}

func TestServiceRecordsTenantFromContext(t *testing.T) {
	input := validRecordInput()
	wantHash := sha256.Sum256(input.RawPayload)
	store := &conversationStoreStub{record: func(_ context.Context, tenantID string, got RecordInput, hash [sha256.Size]byte) (RecordResult, error) {
		if tenantID != conversationTenantID || got.AgentID != input.AgentID || hash != wantHash {
			t.Fatalf("tenant=%q input=%#v hash=%x", tenantID, got, hash)
		}
		return RecordResult{Conversation: Conversation{
			ID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1", TenantID: tenantID,
			Provider: ProviderElevenLabs, ProviderConversationID: input.ProviderConversationID,
		}}, nil
	}}
	result, err := NewService(store).RecordPostCall(tenant.WithID(context.Background(), conversationTenantID), input)
	if err != nil || result.Conversation.TenantID != conversationTenantID {
		t.Fatalf("RecordPostCall() = %#v, %v", result, err)
	}
}

func TestServiceRejectsMissingTenantAndInvalidInput(t *testing.T) {
	store := &conversationStoreStub{record: func(context.Context, string, RecordInput, [sha256.Size]byte) (RecordResult, error) {
		t.Fatal("invalid request reached store")
		return RecordResult{}, nil
	}}
	service := NewService(store)
	if _, err := service.RecordPostCall(context.Background(), validRecordInput()); err == nil {
		t.Fatal("missing tenant succeeded")
	}

	tests := []struct {
		name   string
		mutate func(*RecordInput)
	}{
		{"agent", func(v *RecordInput) { v.AgentID = "" }},
		{"conversation", func(v *RecordInput) { v.ProviderConversationID = "" }},
		{"status", func(v *RecordInput) { v.ProviderStatus = "" }},
		{"event time", func(v *RecordInput) { v.ProviderEventAt = time.Time{} }},
		{"start time", func(v *RecordInput) { v.StartedAt = time.Time{} }},
		{"duration", func(v *RecordInput) { v.DurationSeconds = 86401 }},
		{"cost", func(v *RecordInput) { cost := int64(-1); v.CostFiatMicroUSD = &cost }},
		{"transcript", func(v *RecordInput) { v.Transcript = []byte(`{}`) }},
		{"analysis", func(v *RecordInput) { v.Analysis = []byte(`[]`) }},
		{"metadata", func(v *RecordInput) { v.Metadata = []byte(`[]`) }},
		{"payload", func(v *RecordInput) { v.RawPayload = []byte(`no`) }},
	}
	ctx := tenant.WithID(context.Background(), conversationTenantID)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRecordInput()
			test.mutate(&input)
			_, err := service.RecordPostCall(ctx, input)
			var validation *domain.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error=%v, want ValidationError", err)
			}
		})
	}
	if store.called {
		t.Fatal("invalid request reached store")
	}
}

func TestServiceRejectsCrossTenantStoreResult(t *testing.T) {
	store := &conversationStoreStub{record: func(context.Context, string, RecordInput, [sha256.Size]byte) (RecordResult, error) {
		return RecordResult{Conversation: Conversation{
			ID:       "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1",
			TenantID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea2",
			Provider: ProviderElevenLabs,
		}}, nil
	}}
	_, err := NewService(store).RecordPostCall(tenant.WithID(context.Background(), conversationTenantID), validRecordInput())
	if err == nil {
		t.Fatal("cross-tenant store result succeeded")
	}
}
