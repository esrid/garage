package voice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
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
	w.Header().Set("Cache-Control", "no-store")
	ctx, err := h.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeLookupJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeLookupJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		writeLookupJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLookupBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input lookupRequest
	if err := decoder.Decode(&input); err != nil {
		writeLookupJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request"})
		return
	}
	if err := ensureJSONEnd(decoder); err != nil {
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

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeLookupJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
