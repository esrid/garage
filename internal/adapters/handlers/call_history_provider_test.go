package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/followup"
)

type conversationHistoryStub struct {
	day   conversation.HistoryDay
	entry conversation.HistoryEntry
	err   error
}

func (s conversationHistoryStub) Day(context.Context, time.Time) (conversation.HistoryDay, error) {
	return s.day, s.err
}

func (s conversationHistoryStub) Call(context.Context, string) (conversation.HistoryEntry, error) {
	return s.entry, s.err
}

func TestCallHistoryProviderMapsTimezoneAndTranscript(t *testing.T) {
	location, err := time.LoadLocation("America/Martinique")
	if err != nil {
		t.Fatal(err)
	}
	startedUTC := time.Date(2026, 7, 30, 13, 5, 0, 0, time.UTC)
	entry := conversation.Conversation{
		ID: "call-1", StartedAt: startedUTC, DurationSeconds: 75,
		ProviderStatus: "done", ProviderOutcome: "booked", Summary: "Vidange demandée",
		Transcript: []byte(`[{"role":"user","message":"Bonjour","time_in_call_secs":1.25},{"role":"agent","message":"Bonjour, comment puis-je aider ?"}]`),
	}
	provider := NewCallHistoryProvider(conversationHistoryStub{
		day: conversation.HistoryDay{
			Date: time.Date(2026, 7, 30, 0, 0, 0, 0, location), Timezone: location.String(),
			Conversations: []conversation.Conversation{entry},
		},
		entry: conversation.HistoryEntry{Conversation: entry, Timezone: location.String()},
	}, nil)

	history, err := provider.Calls(context.Background(), startedUTC)
	if err != nil {
		t.Fatalf("Calls() error = %v", err)
	}
	if history.Timezone != location.String() || len(history.Calls) != 1 {
		t.Fatalf("history = %#v", history)
	}
	call := history.Calls[0]
	if call.At.Location().String() != location.String() || call.At.Hour() != 9 || call.Duration != 75*time.Second || call.CustomerName != "" || call.Phone != "" {
		t.Fatalf("call = %#v", call)
	}

	detail, err := provider.Call(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if len(detail.Turns) != 2 || detail.Turns[0].At != 1250*time.Millisecond || detail.Turns[0].Text != "Bonjour" || detail.Turns[1].At != 0 {
		t.Fatalf("turns = %#v", detail.Turns)
	}
}

func TestCallHistoryProviderRejectsInvalidStoredTranscriptAndTimezone(t *testing.T) {
	tests := []struct {
		name  string
		entry conversation.HistoryEntry
	}{
		{"transcript shape", conversation.HistoryEntry{Conversation: conversation.Conversation{Transcript: []byte(`{}`)}, Timezone: "America/Martinique"}},
		{"transcript offset", conversation.HistoryEntry{Conversation: conversation.Conversation{Transcript: []byte(`[{"time_in_call_secs":-1}]`)}, Timezone: "America/Martinique"}},
		{"timezone", conversation.HistoryEntry{Conversation: conversation.Conversation{Transcript: []byte(`[]`)}, Timezone: "Not/A_Zone"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCallHistoryProvider(conversationHistoryStub{entry: test.entry}, nil).Call(context.Background(), "call")
			if err == nil {
				t.Fatal("invalid stored value succeeded")
			}
		})
	}
}

type callerDirectoryStub struct {
	callers map[string]followup.Caller
	err     error
	asked   []string
}

func (s *callerDirectoryStub) Callers(_ context.Context, ids []string) (map[string]followup.Caller, error) {
	s.asked = append(s.asked, ids...)
	return s.callers, s.err
}

// The provider payload documents no caller number, so identity comes from what
// our own voice tools recorded during the call (F08 follow-up requests).
func TestCallHistoryResolvesTheCallerFromOurOwnRecords(t *testing.T) {
	location, err := time.LoadLocation("America/Martinique")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	known := conversation.Conversation{
		ID: "c1", TenantID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1", Provider: conversation.ProviderElevenLabs,
		ProviderConversationID: "conv_known", StartedAt: time.Now(), DurationSeconds: 30,
	}
	unknown := conversation.Conversation{
		ID: "c2", TenantID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1", Provider: conversation.ProviderElevenLabs,
		ProviderConversationID: "conv_unknown", StartedAt: time.Now(), DurationSeconds: 12,
	}
	directory := &callerDirectoryStub{callers: map[string]followup.Caller{
		"conv_known": {Phone: "+596696000001", CustomerName: "Marie Lubin"},
	}}
	provider := NewCallHistoryProvider(conversationHistoryStub{
		day: conversation.HistoryDay{
			Date: time.Date(2026, 7, 30, 0, 0, 0, 0, location), Timezone: location.String(),
			Conversations: []conversation.Conversation{known, unknown},
		},
	}, directory)

	history, err := provider.Calls(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Calls() error = %v", err)
	}
	if history.Calls[0].CustomerName != "Marie Lubin" || history.Calls[0].Phone != "+596696000001" {
		t.Errorf("known caller = %+v, want the recorded identity", history.Calls[0])
	}
	// Nothing invented for a call we hold no record of.
	if history.Calls[1].CustomerName != "" || history.Calls[1].Phone != "" {
		t.Errorf("unknown caller = %+v, want empty identity", history.Calls[1])
	}
	// One lookup for the whole page, not one per row.
	if len(directory.asked) != 2 {
		t.Errorf("directory asked for %v, want both ids in a single call", directory.asked)
	}
}

// A caller lookup failure costs a name, not the day.
func TestCallHistorySurvivesACallerLookupFailure(t *testing.T) {
	location, err := time.LoadLocation("America/Martinique")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	entry := conversation.Conversation{
		ID: "c1", TenantID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1", Provider: conversation.ProviderElevenLabs,
		ProviderConversationID: "conv_1", StartedAt: time.Now(), DurationSeconds: 30,
	}
	provider := NewCallHistoryProvider(conversationHistoryStub{
		day: conversation.HistoryDay{
			Date: time.Date(2026, 7, 30, 0, 0, 0, 0, location), Timezone: location.String(),
			Conversations: []conversation.Conversation{entry},
		},
	}, &callerDirectoryStub{err: errors.New("database is down")})

	history, err := provider.Calls(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Calls() error = %v: a caller lookup failure must not lose the day", err)
	}
	if len(history.Calls) != 1 {
		t.Fatalf("got %d calls, want the day to still render", len(history.Calls))
	}
}
