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
