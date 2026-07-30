package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/web/views"
)

// CallHistoryProvider maps the provider-neutral conversation domain into F15's
// frozen presentation DTO. Provider transcript JSON stops here.
type CallHistoryProvider struct {
	reader conversation.HistoryReader
}

func NewCallHistoryProvider(reader conversation.HistoryReader) *CallHistoryProvider {
	return &CallHistoryProvider{reader: reader}
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
	for _, entry := range history.Conversations {
		result.Calls = append(result.Calls, callSummary(entry, location))
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
	return views.CallDetail{
		CallSummary: callSummary(entry.Conversation, location),
		Turns:       turns,
	}, nil
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
