package followup

import (
	"errors"
	"time"
)

type Kind string

const (
	KindCallback Kind = "callback"
	KindQuote    Kind = "quote"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

var ErrIdempotencyConflict = errors.New("follow-up request reused with different content")

type Request struct {
	ID             string
	TenantID       string
	CustomerID     string
	Kind           Kind
	Phone          string
	Details        string
	Status         Status
	ConversationID string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateInput struct {
	ConversationID string
	Kind           Kind
	Phone          string
	Details        string
}
