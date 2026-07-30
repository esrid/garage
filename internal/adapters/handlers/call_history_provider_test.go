package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/web/views"
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
	})

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
			_, err := NewCallHistoryProvider(conversationHistoryStub{entry: test.entry}).Call(context.Background(), "call")
			if err == nil {
				t.Fatal("invalid stored value succeeded")
			}
		})
	}
}

func TestTodayWithCallsProviderFillsDashboardWithoutInventingSubject(t *testing.T) {
	day := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	base := &stubProvider{data: views.Today{Day: day, Calls: []views.Call{{ID: "fixture"}}}}
	calls := callHistoryReaderStub{history: views.CallHistory{Calls: []views.CallSummary{{
		ID: "call-1", At: day.Add(9 * time.Hour), Duration: 2 * time.Minute,
		Outcome: "transferred", Summary: "Résumé assistant non vérifié",
	}}}}
	result, err := NewTodayWithCallsProvider(base, calls).Today(context.Background(), day)
	if err != nil {
		t.Fatalf("Today() error = %v", err)
	}
	if len(result.Calls) != 1 || result.Calls[0].ID != "call-1" || !result.Calls[0].Transferred || result.Calls[0].Subject != "" {
		t.Fatalf("dashboard calls = %#v", result.Calls)
	}
}

func TestTodayWithCallsProviderPropagatesReadFailure(t *testing.T) {
	want := errors.New("database down")
	_, err := NewTodayWithCallsProvider(&stubProvider{}, callHistoryReaderStub{err: want}).Today(context.Background(), time.Now())
	if !errors.Is(err, want) {
		t.Fatalf("Today() error = %v, want %v", err, want)
	}
}

type callHistoryReaderStub struct {
	history views.CallHistory
	err     error
}

func (s callHistoryReaderStub) Calls(context.Context, time.Time) (views.CallHistory, error) {
	return s.history, s.err
}

func (s callHistoryReaderStub) Call(context.Context, string) (views.CallDetail, error) {
	return views.CallDetail{}, s.err
}
