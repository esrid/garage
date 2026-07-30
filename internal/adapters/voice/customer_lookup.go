package voice

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
)

const maxLookupBodyBytes = 8 << 10

type CustomerLookup struct {
	customers     *customer.Service
	authenticator *TokenAuthenticator
}

type lookupRequest struct {
	Phone string `json:"phone"`
}

type lookupResponse struct {
	Found    bool            `json:"found"`
	Customer *lookupCustomer `json:"customer,omitempty"`
}

type lookupCustomer struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
}

func NewCustomerLookup(customers *customer.Service, authenticator *TokenAuthenticator) *CustomerLookup {
	return &CustomerLookup{customers: customers, authenticator: authenticator}
}

func (h *CustomerLookup) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input lookupRequest
	ctx, tenantID, err := decodeToolRequest(w, r, h.authenticator, maxLookupBodyBytes, &input)
	if err != nil {
		if errors.Is(err, errToolUnauthorized) {
			writeLookupJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		writeLookupJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request"})
		return
	}

	found, err := h.customers.FindByPhone(ctx, input.Phone)
	if err != nil {
		var notFound *domain.NotFoundError
		var validation *domain.ValidationError
		switch {
		case errors.As(err, &notFound):
			writeLookupJSON(w, http.StatusOK, lookupResponse{Found: false})
		case errors.As(err, &validation):
			writeLookupJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request"})
		default:
			writeLookupJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service unavailable"})
		}
		return
	}
	if found.TenantID != tenantID {
		writeLookupJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service unavailable"})
		return
	}
	writeLookupJSON(w, http.StatusOK, lookupResponse{
		Found: true,
		Customer: &lookupCustomer{
			ID:        found.ID,
			FirstName: found.FirstName,
		},
	})
}

func writeLookupJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
