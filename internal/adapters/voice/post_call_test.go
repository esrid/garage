package voice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	postCallSecret = "post-call-webhook-secret-0123456789"
	postCallAgent  = "agent_123"
	postCallNow    = int64(1739537300)
)

type postCallRecorderStub struct {
	record func(context.Context, conversation.RecordInput) (conversation.RecordResult, error)
	called bool
}

func (s *postCallRecorderStub) RecordPostCall(ctx context.Context, input conversation.RecordInput) (conversation.RecordResult, error) {
	s.called = true
	return s.record(ctx, input)
}

func postCallBody(agentID string) string {
	return `{"type":"post_call_transcription","event_timestamp":1739537297,"data":{` +
		`"agent_id":"` + agentID + `","conversation_id":"conv_123","status":"done",` +
		`"transcript":[{"role":"user","message":"Bonjour","time_in_call_secs":2}],` +
		`"metadata":{"start_time_unix_secs":1739537275,"call_duration_secs":22,"cost":296,"cost_fiat":1.1},` +
		`"analysis":{"call_successful":"success","transcript_summary":"Rendez-vous demandé."},` +
		`"future_field":{"kept":true}}}`
}

func signedPostCallRequest(body, secret string, timestamp int64) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/elevenlabs/post-call", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	messageTimestamp := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(messageTimestamp + "." + body))
	request.Header.Set("ElevenLabs-Signature", "t="+messageTimestamp+",v0="+hex.EncodeToString(mac.Sum(nil)))
	return request
}

func newPostCallHandler(t *testing.T, recorder *postCallRecorderStub) *PostCallWebhook {
	t.Helper()
	handler, err := NewPostCallWebhook(recorder, postCallSecret, postCallAgent+":"+voiceTenantA)
	if err != nil {
		t.Fatalf("NewPostCallWebhook() error = %v", err)
	}
	handler.now = func() time.Time { return time.Unix(postCallNow, 0) }
	return handler
}

func TestPostCallWebhookAuthenticatesMapsTenantAndRecords(t *testing.T) {
	recorder := &postCallRecorderStub{record: func(ctx context.Context, input conversation.RecordInput) (conversation.RecordResult, error) {
		tenantID, err := tenant.IDFromContext(ctx)
		if err != nil || tenantID != voiceTenantA {
			t.Fatalf("tenant=%q err=%v", tenantID, err)
		}
		if input.AgentID != postCallAgent || input.ProviderConversationID != "conv_123" || input.DurationSeconds != 22 || input.CostFiatMicroUSD == nil || *input.CostFiatMicroUSD != 1_100_000 || input.ProviderOutcome != "success" || input.Summary != "Rendez-vous demandé." {
			t.Fatalf("input=%#v", input)
		}
		if !strings.Contains(string(input.RawPayload), `"future_field"`) || !strings.Contains(string(input.Metadata), `"cost":296`) {
			t.Fatalf("raw provider fields were not retained: %s / %s", input.RawPayload, input.Metadata)
		}
		return conversation.RecordResult{Conversation: conversation.Conversation{
			ID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1", TenantID: tenantID,
			Provider: conversation.ProviderElevenLabs, ProviderConversationID: input.ProviderConversationID,
		}}, nil
	}}
	body := postCallBody(postCallAgent)
	response := httptest.NewRecorder()
	newPostCallHandler(t, recorder).ServeHTTP(response, signedPostCallRequest(body, postCallSecret, postCallNow))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"status":"received"}` {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response is cacheable")
	}
}

func TestPostCallWebhookRejectsUntrustedOrInvalidDeliveries(t *testing.T) {
	recorder := &postCallRecorderStub{record: func(context.Context, conversation.RecordInput) (conversation.RecordResult, error) {
		t.Fatal("invalid delivery reached recorder")
		return conversation.RecordResult{}, nil
	}}
	handler := newPostCallHandler(t, recorder)
	body := postCallBody(postCallAgent)
	tests := []struct {
		name    string
		request func() *http.Request
		want    int
	}{
		{"missing signature", func() *http.Request {
			r := signedPostCallRequest(body, postCallSecret, postCallNow)
			r.Header.Del("ElevenLabs-Signature")
			return r
		}, http.StatusUnauthorized},
		{"wrong signature", func() *http.Request { return signedPostCallRequest(body, strings.Repeat("x", 32), postCallNow) }, http.StatusUnauthorized},
		{"modified body", func() *http.Request {
			r := signedPostCallRequest(body, postCallSecret, postCallNow)
			r.Body = io.NopCloser(strings.NewReader(body + " "))
			return r
		}, http.StatusUnauthorized},
		{"stale signature", func() *http.Request { return signedPostCallRequest(body, postCallSecret, postCallNow-1801) }, http.StatusUnauthorized},
		{"media type", func() *http.Request {
			r := signedPostCallRequest(body, postCallSecret, postCallNow)
			r.Header.Set("Content-Type", "text/plain")
			return r
		}, http.StatusUnsupportedMediaType},
		{"unknown agent", func() *http.Request {
			unknown := postCallBody("agent_unknown")
			return signedPostCallRequest(unknown, postCallSecret, postCallNow)
		}, http.StatusBadRequest},
		{"unsupported event", func() *http.Request {
			unsupported := strings.Replace(body, "post_call_transcription", "post_call_audio", 1)
			return signedPostCallRequest(unsupported, postCallSecret, postCallNow)
		}, http.StatusBadRequest},
		{"malformed", func() *http.Request { return signedPostCallRequest(`{`, postCallSecret, postCallNow) }, http.StatusBadRequest},
		{"oversized", func() *http.Request {
			large := strings.Repeat("x", maxPostCallBodyBytes+1)
			return signedPostCallRequest(large, postCallSecret, postCallNow)
		}, http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, test.request())
			if response.Code != test.want || !strings.Contains(response.Body.String(), `"error"`) {
				t.Fatalf("status=%d body=%q, want %d", response.Code, response.Body.String(), test.want)
			}
			for _, secret := range []string{voiceTenantA, postCallSecret, postCallAgent, "conv_123"} {
				if strings.Contains(response.Body.String(), secret) {
					t.Fatalf("response leaked %q: %s", secret, response.Body.String())
				}
			}
		})
	}
	if recorder.called {
		t.Fatal("invalid delivery reached recorder")
	}
}

func TestPostCallWebhookMapsRecorderFailures(t *testing.T) {
	tests := []struct {
		name   string
		result conversation.RecordResult
		err    error
		want   int
	}{
		{"conflict", conversation.RecordResult{}, conversation.ErrEventConflict, http.StatusConflict},
		{"database", conversation.RecordResult{}, errors.New("database secret"), http.StatusServiceUnavailable},
		{"cross tenant", conversation.RecordResult{Conversation: conversation.Conversation{ID: "id", TenantID: voiceTenantB, ProviderConversationID: "conv_123"}}, nil, http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &postCallRecorderStub{record: func(context.Context, conversation.RecordInput) (conversation.RecordResult, error) {
				return test.result, test.err
			}}
			response := httptest.NewRecorder()
			newPostCallHandler(t, recorder).ServeHTTP(response, signedPostCallRequest(postCallBody(postCallAgent), postCallSecret, postCallNow))
			if response.Code != test.want || strings.Contains(response.Body.String(), "database") || strings.Contains(response.Body.String(), voiceTenantB) {
				t.Fatalf("status=%d body=%q, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

func TestNewPostCallWebhookValidatesConfiguration(t *testing.T) {
	recorder := &postCallRecorderStub{}
	disabled, err := NewPostCallWebhook(recorder, "", "")
	if err != nil {
		t.Fatalf("disabled constructor error = %v", err)
	}
	response := httptest.NewRecorder()
	disabled.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/webhooks/elevenlabs/post-call", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled status=%d", response.Code)
	}

	invalid := []struct{ secret, mappings string }{
		{postCallSecret, ""},
		{"", postCallAgent + ":" + voiceTenantA},
		{"short", postCallAgent + ":" + voiceTenantA},
		{postCallSecret, "bad"},
		{postCallSecret, postCallAgent + ":bad-tenant"},
		{postCallSecret, postCallAgent + ":" + voiceTenantA + "," + postCallAgent + ":" + voiceTenantB},
	}
	for _, item := range invalid {
		if _, err := NewPostCallWebhook(recorder, item.secret, item.mappings); err == nil {
			t.Errorf("NewPostCallWebhook(%q, %q) succeeded", item.secret, item.mappings)
		}
	}
}

func TestParseMicroUSD(t *testing.T) {
	for input, want := range map[string]int64{
		"1.1": 1_100_000, "0": 0, "1e-6": 1,
		"0.0012345": 1235, "0.0000004": 0, "0.0000005": 1,
	} {
		number := json.Number(input)
		got, err := parseMicroUSD(&number)
		if err != nil || got == nil || *got != want {
			t.Errorf("parseMicroUSD(%q)=%v,%v want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"-1", "1000001"} {
		number := json.Number(input)
		if _, err := parseMicroUSD(&number); err == nil {
			t.Errorf("parseMicroUSD(%q) succeeded", input)
		}
	}
}
