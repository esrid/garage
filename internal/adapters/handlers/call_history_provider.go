package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/web/views"
)

// CallHistoryProvider maps the provider-neutral conversation domain into F15's
// frozen presentation DTO. Provider transcript JSON stops here.
//
// Caller identity is composed here rather than joined in the conversation read
// model: who called is something our own voice tools recorded (F08), not
// something the conversation domain knows. Keeping the two apart means neither
// domain has to learn about the other.
type CallHistoryProvider struct {
	reader  conversation.HistoryReader
	callers followup.CallerDirectory
}

// NewCallHistoryProvider builds the adapter. callers may be nil: the history then
// renders what the provider gave, and rows are titled by the hour instead of by a
// name we do not have.
func NewCallHistoryProvider(reader conversation.HistoryReader, callers followup.CallerDirectory) *CallHistoryProvider {
	return &CallHistoryProvider{reader: reader, callers: callers}
}

func (p *CallHistoryProvider) Calls(ctx context.Context, day time.Time) (views.CallHistory, error) {
	history, err := p.reader.Day(ctx, day)
	if err != nil {
		return views.CallHistory{}, err
	}
	location, err := time.LoadLocation(history.Timezone)
	if err != nil {
		return views.CallHistory{}, fmt.Errorf("call history: load tenant timezone: %w", err)
	}
	result := views.CallHistory{
		Day:      history.Date.In(location),
		Timezone: history.Timezone,
		Calls:    make([]views.CallSummary, 0, len(history.Conversations)),
	}
	identities := p.identify(ctx, conversationIDs(history.Conversations))
	for _, entry := range history.Conversations {
		summary := callSummary(entry, location)
		applyCaller(&summary, identities[entry.ProviderConversationID])
		result.Calls = append(result.Calls, summary)
	}
	return result, nil
}

func (p *CallHistoryProvider) Call(ctx context.Context, id string) (views.CallDetail, error) {
	entry, err := p.reader.Call(ctx, id)
	if err != nil {
		return views.CallDetail{}, err
	}
	location, err := time.LoadLocation(entry.Timezone)
	if err != nil {
		return views.CallDetail{}, fmt.Errorf("call history: load tenant timezone: %w", err)
	}
	turns, err := transcriptTurns(entry.Conversation.Transcript)
	if err != nil {
		return views.CallDetail{}, fmt.Errorf("call history: map transcript: %w", err)
	}
	summary := callSummary(entry.Conversation, location)
	applyCaller(&summary, p.identify(ctx, []string{entry.Conversation.ProviderConversationID})[entry.Conversation.ProviderConversationID])
	return views.CallDetail{CallSummary: summary, Turns: turns}, nil
}

// identify resolves who called, and never fails the page: an unresolved caller
// costs a name, a failed history costs the whole day. The reason is logged.
func (p *CallHistoryProvider) identify(ctx context.Context, ids []string) map[string]followup.Caller {
	if p.callers == nil || len(ids) == 0 {
		return nil
	}
	identities, err := p.callers.Callers(ctx, ids)
	if err != nil {
		slog.ErrorContext(ctx, "call history: caller identity unavailable", "err", err)
		return nil
	}
	return identities
}

func conversationIDs(entries []conversation.Conversation) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ProviderConversationID)
	}
	return ids
}

func applyCaller(summary *views.CallSummary, caller followup.Caller) {
	summary.Phone = caller.Phone
	summary.CustomerName = caller.CustomerName
}

func callSummary(entry conversation.Conversation, location *time.Location) views.CallSummary {
	return views.CallSummary{
		ID:       entry.ID,
		At:       entry.StartedAt.In(location),
		Duration: time.Duration(entry.DurationSeconds) * time.Second,
		Outcome:  entry.ProviderOutcome,
		Status:   entry.ProviderStatus,
		Summary:  entry.Summary,
	}
}

type providerTranscriptTurn struct {
	Role           string      `json:"role"`
	Message        string      `json:"message"`
	TimeInCallSecs json.Number `json:"time_in_call_secs"`
}

func transcriptTurns(raw []byte) ([]views.CallTurn, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("transcript is not an array")
	}
	var providerTurns []providerTranscriptTurn
	if err := json.Unmarshal(trimmed, &providerTurns); err != nil {
		return nil, err
	}
	result := make([]views.CallTurn, 0, len(providerTurns))
	for _, turn := range providerTurns {
		offset, err := transcriptOffset(turn.TimeInCallSecs)
		if err != nil {
			return nil, err
		}
		result = append(result, views.CallTurn{Role: turn.Role, Text: turn.Message, At: offset})
	}
	return result, nil
}

func transcriptOffset(value json.Number) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	seconds, err := strconv.ParseFloat(string(value), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 24*60*60 {
		return 0, fmt.Errorf("invalid transcript offset %q", value)
	}
	return time.Duration(math.Round(seconds * float64(time.Second))), nil
}
