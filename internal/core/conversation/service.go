package conversation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const maxRawPayloadBytes = 2 << 20

type Store interface {
	RecordPostCall(context.Context, string, RecordInput, [sha256.Size]byte) (RecordResult, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) RecordPostCall(ctx context.Context, input RecordInput) (RecordResult, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return RecordResult{}, err
	}
	if err := normalizeAndValidate(&input); err != nil {
		return RecordResult{}, err
	}

	result, err := s.store.RecordPostCall(ctx, tenantID, input, sha256.Sum256(input.RawPayload))
	if err != nil {
		return RecordResult{}, err
	}
	if result.Conversation.ID == "" || result.Conversation.TenantID != tenantID || result.Conversation.Provider != ProviderElevenLabs {
		return RecordResult{}, fmt.Errorf("conversation store returned an invalid result")
	}
	return result, nil
}

func normalizeAndValidate(input *RecordInput) error {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.ProviderConversationID = strings.TrimSpace(input.ProviderConversationID)
	input.ProviderStatus = strings.TrimSpace(input.ProviderStatus)
	input.Summary = strings.TrimSpace(input.Summary)
	input.ProviderOutcome = strings.TrimSpace(input.ProviderOutcome)

	invalid := make(map[string]string)
	validateText(invalid, "agent_id", input.AgentID, 512, false)
	validateText(invalid, "conversation_id", input.ProviderConversationID, 512, false)
	validateText(invalid, "status", input.ProviderStatus, 64, false)
	validateText(invalid, "summary", input.Summary, 10000, true)
	validateText(invalid, "provider_outcome", input.ProviderOutcome, 128, true)
	if input.ProviderEventAt.IsZero() {
		invalid["event_timestamp"] = "invalid"
	}
	if input.StartedAt.IsZero() {
		invalid["start_time"] = "invalid"
	}
	if input.DurationSeconds < 0 || input.DurationSeconds > 24*60*60 {
		invalid["duration"] = "invalid"
	}
	if input.CostFiatMicroUSD != nil && (*input.CostFiatMicroUSD < 0 || *input.CostFiatMicroUSD > 1_000_000_000_000) {
		invalid["cost_fiat"] = "invalid"
	}
	if !validJSONKind(input.Transcript, '[') {
		invalid["transcript"] = "invalid"
	}
	if !validJSONKind(input.Metadata, '{') {
		invalid["metadata"] = "invalid"
	}
	if !validJSONObjectOrNull(input.Analysis) {
		invalid["analysis"] = "invalid"
	}
	if len(input.RawPayload) == 0 || len(input.RawPayload) > maxRawPayloadBytes || !json.Valid(input.RawPayload) {
		invalid["payload"] = "invalid"
	}
	if len(invalid) != 0 {
		return &domain.ValidationError{Entity: "post-call event", Errors: invalid}
	}
	return nil
}

func validateText(invalid map[string]string, field, value string, limit int, optional bool) {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit || (!optional && value == "") {
		invalid[field] = "invalid"
	}
}

func validJSONKind(value []byte, want byte) bool {
	trimmed := bytes.TrimSpace(value)
	return len(trimmed) != 0 && trimmed[0] == want && json.Valid(trimmed)
}

func validJSONObjectOrNull(value []byte) bool {
	trimmed := bytes.TrimSpace(value)
	return bytes.Equal(trimmed, []byte("null")) || validJSONKind(trimmed, '{')
}
