package voice

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	maxPostCallBodyBytes = 2 << 20
	webhookPastTolerance = 30 * time.Minute
	// Clock skew between the provider and this server, not a replay window.
	webhookFutureTolerance = 5 * time.Minute
	maxAgentIDLength       = 512
	minWebhookSecretBytes  = 16
	maxWebhookSecretBytes  = 512
	maxCostFiatMicroUSD    = int64(1_000_000_000_000)
)

type postCallRecorder interface {
	RecordPostCall(context.Context, conversation.RecordInput) (conversation.RecordResult, error)
}

type PostCallWebhook struct {
	recorder      postCallRecorder
	secret        []byte
	tenantByAgent map[string]string
	now           func() time.Time
	enabled       bool
}

type postCallEvent struct {
	Type           string       `json:"type"`
	EventTimestamp json.Number  `json:"event_timestamp"`
	Data           postCallData `json:"data"`
}

type postCallData struct {
	AgentID        string          `json:"agent_id"`
	ConversationID string          `json:"conversation_id"`
	Status         string          `json:"status"`
	Transcript     json.RawMessage `json:"transcript"`
	Metadata       json.RawMessage `json:"metadata"`
	Analysis       json.RawMessage `json:"analysis"`
}

type postCallMetadata struct {
	StartTimeUnixSeconds json.Number  `json:"start_time_unix_secs"`
	CallDurationSeconds  json.Number  `json:"call_duration_secs"`
	CostFiat             *json.Number `json:"cost_fiat"`
}

type postCallAnalysis struct {
	TranscriptSummary string `json:"transcript_summary"`
	CallSuccessful    string `json:"call_successful"`
}

func NewPostCallWebhook(recorder postCallRecorder, secret, encodedAgentTenants string) (*PostCallWebhook, error) {
	handler := &PostCallWebhook{
		recorder:      recorder,
		tenantByAgent: make(map[string]string),
		now:           time.Now,
	}
	if secret == "" && strings.TrimSpace(encodedAgentTenants) == "" {
		return handler, nil
	}
	if secret == "" || strings.TrimSpace(encodedAgentTenants) == "" {
		return nil, fmt.Errorf("post-call webhook: secret and agent mapping must be configured together")
	}
	if len(secret) < minWebhookSecretBytes || len(secret) > maxWebhookSecretBytes {
		return nil, fmt.Errorf("post-call webhook: secret must contain 16 to 512 bytes")
	}

	for item := range strings.SplitSeq(encodedAgentTenants, ",") {
		if strings.Count(item, ":") != 1 {
			return nil, fmt.Errorf("post-call webhook: each mapping must be agent-id:tenant-uuid")
		}
		agentID, tenantID, _ := strings.Cut(item, ":")
		agentID = strings.TrimSpace(agentID)
		tenantID = strings.ToLower(strings.TrimSpace(tenantID))
		if !validAgentID(agentID) {
			return nil, fmt.Errorf("post-call webhook: invalid agent ID")
		}
		if !validUUID(tenantID) {
			return nil, fmt.Errorf("post-call webhook: tenant ID must be a UUID")
		}
		if _, exists := handler.tenantByAgent[agentID]; exists {
			return nil, fmt.Errorf("post-call webhook: duplicate agent ID")
		}
		handler.tenantByAgent[agentID] = tenantID
	}
	handler.secret = []byte(secret)
	handler.enabled = true
	return handler, nil
}

// Register mounts the post-call webhook. Its boundary is an HMAC signature over
// the raw body, verified inside before anything is parsed.
func (h *PostCallWebhook) Register(mux *http.ServeMux) {
	mux.Handle("POST /webhooks/elevenlabs/post-call", h)
}

func (h *PostCallWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.enabled {
		writePostCallError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPostCallBodyBytes)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writePostCallError(w, http.StatusRequestEntityTooLarge, "event too large")
			return
		}
		writePostCallError(w, http.StatusBadRequest, "invalid event")
		return
	}
	if !h.validSignature(rawBody, r.Header.Get("ElevenLabs-Signature")) {
		writePostCallError(w, http.StatusUnauthorized, "invalid signature")
		return
	}
	if !isMediaType(r.Header.Get("Content-Type"), "application/json") {
		writePostCallError(w, http.StatusUnsupportedMediaType, "unsupported media type")
		return
	}

	event, input, err := decodePostCallEvent(rawBody)
	if err != nil {
		writePostCallError(w, http.StatusBadRequest, "invalid event")
		return
	}
	tenantID, exists := h.tenantByAgent[event.Data.AgentID]
	if !exists {
		writePostCallError(w, http.StatusBadRequest, "invalid event")
		return
	}

	result, err := h.recorder.RecordPostCall(tenant.WithID(r.Context(), tenantID), input)
	if err != nil {
		var validation *domain.ValidationError
		switch {
		case errors.Is(err, conversation.ErrEventConflict):
			writePostCallError(w, http.StatusConflict, "event conflict")
		case errors.As(err, &validation):
			writePostCallError(w, http.StatusBadRequest, "invalid event")
		default:
			writePostCallError(w, http.StatusServiceUnavailable, "service unavailable")
		}
		return
	}
	if result.Conversation.TenantID != tenantID || result.Conversation.ProviderConversationID != event.Data.ConversationID {
		writePostCallError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	writePostCallJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (h *PostCallWebhook) validSignature(rawBody []byte, header string) bool {
	var timestampValue, signatureValue string
	for part := range strings.SplitSeq(header, ",") {
		// The documented format has no spaces, but a provider that starts sending
		// "t=1,​ v0=..." would otherwise fail every signature at once.
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "t=") && timestampValue == "":
			timestampValue = strings.TrimPrefix(part, "t=")
		case strings.HasPrefix(part, "v0=") && signatureValue == "":
			signatureValue = strings.TrimPrefix(part, "v0=")
		}
	}
	timestamp, err := strconv.ParseInt(timestampValue, 10, 64)
	if err != nil || timestampValue == "" || signatureValue == "" {
		return false
	}
	// Bounded on both sides. The past window is what the contract specifies; the
	// future one closes a signed event with a skewed clock staying replayable long
	// after the 30 minutes are meant to have expired.
	now := h.now()
	if timestamp < now.Add(-webhookPastTolerance).Unix() || timestamp > now.Add(webhookFutureTolerance).Unix() {
		return false
	}
	provided, err := hex.DecodeString(signatureValue)
	if err != nil || len(provided) != sha256.Size || signatureValue != strings.ToLower(signatureValue) {
		return false
	}
	mac := hmac.New(sha256.New, h.secret)
	_, _ = io.WriteString(mac, timestampValue)
	_, _ = mac.Write([]byte{'.'})
	_, _ = mac.Write(rawBody)
	return hmac.Equal(provided, mac.Sum(nil))
}

func decodePostCallEvent(rawBody []byte) (postCallEvent, conversation.RecordInput, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.UseNumber()
	var event postCallEvent
	if err := decoder.Decode(&event); err != nil {
		return event, conversation.RecordInput{}, err
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return event, conversation.RecordInput{}, err
	}
	if event.Type != "post_call_transcription" {
		return event, conversation.RecordInput{}, errors.New("unsupported post-call event")
	}
	eventUnix, err := event.EventTimestamp.Int64()
	if err != nil || eventUnix <= 0 {
		return event, conversation.RecordInput{}, errors.New("invalid event timestamp")
	}

	var metadata postCallMetadata
	metadataDecoder := json.NewDecoder(bytes.NewReader(event.Data.Metadata))
	metadataDecoder.UseNumber()
	if err := metadataDecoder.Decode(&metadata); err != nil {
		return event, conversation.RecordInput{}, err
	}
	startUnix, err := metadata.StartTimeUnixSeconds.Int64()
	if err != nil || startUnix <= 0 {
		return event, conversation.RecordInput{}, errors.New("invalid start timestamp")
	}
	duration, err := metadata.CallDurationSeconds.Int64()
	if err != nil || duration < 0 || duration > 24*60*60 {
		return event, conversation.RecordInput{}, errors.New("invalid call duration")
	}
	cost, err := parseMicroUSD(metadata.CostFiat)
	if err != nil {
		return event, conversation.RecordInput{}, err
	}

	var analysis postCallAnalysis
	if !bytes.Equal(bytes.TrimSpace(event.Data.Analysis), []byte("null")) {
		if err := json.Unmarshal(event.Data.Analysis, &analysis); err != nil {
			return event, conversation.RecordInput{}, err
		}
	}
	return event, conversation.RecordInput{
		AgentID:                event.Data.AgentID,
		ProviderConversationID: event.Data.ConversationID,
		ProviderStatus:         event.Data.Status,
		ProviderEventAt:        time.Unix(eventUnix, 0).UTC(),
		StartedAt:              time.Unix(startUnix, 0).UTC(),
		DurationSeconds:        int(duration),
		CostFiatMicroUSD:       cost,
		Transcript:             append([]byte(nil), event.Data.Transcript...),
		Summary:                analysis.TranscriptSummary,
		ProviderOutcome:        analysis.CallSuccessful,
		Analysis:               append([]byte(nil), event.Data.Analysis...),
		Metadata:               append([]byte(nil), event.Data.Metadata...),
		RawPayload:             append([]byte(nil), rawBody...),
	}, nil
}

func parseMicroUSD(number *json.Number) (*int64, error) {
	if number == nil {
		return nil, nil
	}
	value, ok := new(big.Rat).SetString(number.String())
	if !ok || value.Sign() < 0 {
		return nil, errors.New("invalid fiat cost")
	}
	value.Mul(value, big.NewRat(1_000_000, 1))
	microUSD := new(big.Int)
	remainder := new(big.Int)
	microUSD.QuoRem(value.Num(), value.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(value.Denom()) >= 0 {
		microUSD.Add(microUSD, big.NewInt(1))
	}
	if !microUSD.IsInt64() || microUSD.Int64() > maxCostFiatMicroUSD {
		return nil, errors.New("fiat cost is too large")
	}
	result := microUSD.Int64()
	return &result, nil
}

func validAgentID(value string) bool {
	if value == "" || len(value) > maxAgentIDLength || strings.ContainsAny(value, ",:") {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func writePostCallError(w http.ResponseWriter, status int, message string) {
	writePostCallJSON(w, status, map[string]string{"error": message})
}

func writePostCallJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
