package voice

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/followup"
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

// Register mounts the follow-up tool.
func (h *FollowUpTool) Register(mux *http.ServeMux) {
	mux.Handle("POST /voice/tools/follow-up-request", h)
}

func (h *FollowUpTool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input followUpToolRequest
	ctx, tenantID, err := decodeToolRequest(w, r, h.authenticator, maxFollowUpBodyBytes, &input)
	if err != nil {
		if errors.Is(err, errToolUnauthorized) {
			writeFollowUpError(w, &domain.UnauthorizedError{Message: "voice tool authentication required"})
			return
		}
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
