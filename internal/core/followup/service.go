package followup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

type Store interface {
	CreateFollowUpRequest(context.Context, string, CreateInput, string) (Request, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Request, error) {
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		return Request{}, err
	}
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.Details = strings.TrimSpace(input.Details)
	if input.ConversationID == "" || !utf8.ValidString(input.ConversationID) || utf8.RuneCountInString(input.ConversationID) > 512 {
		return Request{}, validationError("conversation_id")
	}
	if input.Kind != KindCallback && input.Kind != KindQuote {
		return Request{}, validationError("kind")
	}
	if input.Details == "" || !utf8.ValidString(input.Details) || utf8.RuneCountInString(input.Details) > 1000 {
		return Request{}, validationError("details")
	}
	input.Phone, err = customer.NormalizePhone(input.Phone)
	if err != nil {
		return Request{}, err
	}
	return s.store.CreateFollowUpRequest(ctx, tenantID, input, requestHash(input.Phone, input.Details))
}

func requestHash(phone, details string) string {
	var canonical strings.Builder
	for _, value := range []string{phone, details} {
		canonical.WriteString(strconv.Itoa(len(value)))
		canonical.WriteByte(':')
		canonical.WriteString(value)
	}
	hash := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(hash[:])
}

func validationError(field string) error {
	return &domain.ValidationError{Entity: "follow-up request", Errors: map[string]string{field: "invalid"}}
}
