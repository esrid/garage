package conversation

import (
	"errors"
	"time"
)

const ProviderElevenLabs = "elevenlabs"

var ErrEventConflict = errors.New("conversation event reused with different content")

type Conversation struct {
	ID                     string
	TenantID               string
	Provider               string
	AgentID                string
	ProviderConversationID string
	ProviderStatus         string
	ProviderEventAt        time.Time
	StartedAt              time.Time
	DurationSeconds        int
	CostFiatMicroUSD       *int64
	Transcript             []byte
	Summary                string
	ProviderOutcome        string
	Analysis               []byte
	Metadata               []byte
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type RecordInput struct {
	AgentID                string
	ProviderConversationID string
	ProviderStatus         string
	ProviderEventAt        time.Time
	StartedAt              time.Time
	DurationSeconds        int
	CostFiatMicroUSD       *int64
	Transcript             []byte
	Summary                string
	ProviderOutcome        string
	Analysis               []byte
	Metadata               []byte
	RawPayload             []byte
}

type RecordResult struct {
	Conversation Conversation
	Duplicate    bool
}

// HistoryDay is the persisted conversations whose start falls within one
// civil day of the tenant's timezone.
type HistoryDay struct {
	Date          time.Time
	Timezone      string
	Conversations []Conversation
}

// HistoryEntry carries the tenant timezone needed to render one conversation.
// Timezone is tenant configuration, not provider data.
type HistoryEntry struct {
	Conversation Conversation
	Timezone     string
}
