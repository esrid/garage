package voice

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/core/tenant"
)

const maxFollowUpBodyBytes = 16 << 10

type FollowUpTool struct {
	service       *followup.Service
	authenticator *TokenAuthenticator
}

type followUpToolRequest struct {
	ConversationID string        `json:"conversation_id"`
	Kind           followup.Kind `json:"kind"`
	Phone          string        `json:"phone"`
	Details        string        `json:"details"`
}

type followUpToolResponse struct {
	Recorded bool                     `json:"recorded"`
	Request  *recordedFollowUpRequest `json:"request,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

type recordedFollowUpRequest struct {
	ID     string          `json:"id"`
	Kind   followup.Kind   `json:"kind"`
	Status followup.Status `json:"status"`
}

func NewFollowUpTool(service *followup.Service, authenticator *TokenAuthenticator) *FollowUpTool {
	return &FollowUpTool{service: service, authenticator: authenticator}
}

func (h *FollowUpTool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, err := h.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeFollowUpError(w, err)
		return
	}
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeFollowUpError(w, err)
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType != "application/json" {
		writeFollowUpError(w, followUpValidation())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxFollowUpBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input followUpToolRequest
	if err := decoder.Decode(&input); err != nil {
		writeFollowUpError(w, followUpValidation())
		return
	}
	if err := ensureJSONEnd(decoder); err != nil {
		writeFollowUpError(w, followUpValidation())
		return
	}

	created, err := h.service.Create(ctx, followup.CreateInput{
		ConversationID: input.ConversationID,
		Kind:           input.Kind,
		Phone:          input.Phone,
		Details:        input.Details,
	})
	if err != nil {
		writeFollowUpError(w, err)
		return
	}
	if created.TenantID != tenantID || created.ID == "" || created.Kind != input.Kind || created.Status != followup.StatusPending {
		writeFollowUpError(w, errors.New("follow-up store returned an invalid result"))
		return
	}
	writeFollowUpJSON(w, http.StatusOK, followUpToolResponse{
		Recorded: true,
		Request: &recordedFollowUpRequest{
			ID:     created.ID,
			Kind:   created.Kind,
			Status: created.Status,
		},
	})
}

func followUpValidation() error {
	return &domain.ValidationError{Entity: "follow-up request"}
}

func writeFollowUpError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	message := "service unavailable"
	var unauthorized *domain.UnauthorizedError
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &unauthorized):
		status, message = http.StatusUnauthorized, "authentication required"
	case errors.As(err, &validation):
		status, message = http.StatusUnprocessableEntity, "invalid request"
	case errors.Is(err, followup.ErrIdempotencyConflict):
		status, message = http.StatusConflict, "request conflict"
	}
	writeFollowUpJSON(w, status, followUpToolResponse{Recorded: false, Error: message})
}

func writeFollowUpJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
