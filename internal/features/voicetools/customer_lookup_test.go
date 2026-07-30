package voicetools

import (
	"context"
	"errors"
	"github.com/esrid/garage/internal/adapters/voice"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/domain"
)

type customerStoreStub struct {
	find func(tenantID, phone string) (customer.Customer, error)
}

func (s *customerStoreStub) CreateCustomer(context.Context, string, customer.CreateInput) (customer.Customer, error) {
	return customer.Customer{}, errors.New("not used")
}

func (s *customerStoreStub) FindCustomerByPhone(_ context.Context, tenantID, phone string) (customer.Customer, error) {
	return s.find(tenantID, phone)
}

func newLookupHandler(t *testing.T, store *customerStoreStub) *CustomerLookup {
	t.Helper()
	authenticator, err := voice.NewTokenAuthenticator(voiceTenantA + ":" + voiceTokenA + "," + voiceTenantB + ":" + voiceTokenB)
	if err != nil {
		t.Fatalf("voice.NewTokenAuthenticator() error = %v", err)
	}
	return NewCustomerLookup(customer.NewService(store), authenticator)
}

func newLookupRequest(body, token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/voice/tools/customer-lookup", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func TestCustomerLookupReturnsMinimalKnownCustomer(t *testing.T) {
	store := &customerStoreStub{find: func(tenantID, phone string) (customer.Customer, error) {
		if tenantID != voiceTenantA || phone != "+596696123456" {
			t.Fatalf("store tenant=%q phone=%q", tenantID, phone)
		}
		return customer.Customer{
			ID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79eaf", TenantID: voiceTenantA,
			FirstName: "Ana", LastName: "Césaire", Phone: "+596696123456",
		}, nil
	}}
	response := httptest.NewRecorder()
	newLookupHandler(t, store).ServeHTTP(response, newLookupRequest(`{"phone":"+596 696 12 34 56"}`, voiceTokenA))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`"found":true`, `"id":"019c09ea-bca7-7a5d-98b6-3f3b3ed79eaf"`, `"first_name":"Ana"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q missing %q", body, want)
		}
	}
	for _, secret := range []string{"Césaire", "+596696123456", voiceTenantA, voiceTokenA} {
		if strings.Contains(body, secret) {
			t.Errorf("body leaked %q: %s", secret, body)
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("lookup response is cacheable")
	}
}

func TestCustomerLookupUnknownIsTenantScopedNormalResult(t *testing.T) {
	store := &customerStoreStub{find: func(tenantID, _ string) (customer.Customer, error) {
		if tenantID == voiceTenantB {
			return customer.Customer{ID: "customer-b", FirstName: "Mia"}, nil
		}
		return customer.Customer{}, &domain.NotFoundError{Entity: "customer"}
	}}
	response := httptest.NewRecorder()
	newLookupHandler(t, store).ServeHTTP(response, newLookupRequest(`{"phone":"+596696123456"}`, voiceTokenA))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"found":false}` {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestCustomerLookupRejectsUnauthorizedAndInvalidRequests(t *testing.T) {
	store := &customerStoreStub{find: func(string, string) (customer.Customer, error) {
		t.Fatal("store must not be called")
		return customer.Customer{}, nil
	}}
	handler := newLookupHandler(t, store)
	tests := []struct {
		name        string
		body        string
		token       string
		contentType string
		want        int
	}{
		{"missing auth", `{"phone":"+596696123456"}`, "", "application/json", http.StatusUnauthorized},
		{"wrong auth", `{"phone":"+596696123456"}`, voiceTokenB + "x", "application/json", http.StatusUnauthorized},
		{"wrong content type", `{"phone":"+596696123456"}`, voiceTokenA, "text/plain", http.StatusUnprocessableEntity},
		{"malformed", `{`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"unknown field", `{"phone":"+596696123456","tenant_id":"` + voiceTenantB + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"multiple values", `{"phone":"+596696123456"}{}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"invalid phone", `{"phone":"0696123456"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"oversized", `{"phone":"+596` + strings.Repeat("1", maxLookupBodyBytes) + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newLookupRequest(test.body, test.token)
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%q", response.Code, test.want, response.Body.String())
			}
			if test.want == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("401 response is missing the Bearer challenge")
			}
			if strings.Contains(response.Body.String(), voiceTokenA) || strings.Contains(response.Body.String(), voiceTenantA) {
				t.Fatal("error response leaked credentials or tenant")
			}
		})
	}
}

func TestCustomerLookupHidesStoreFailure(t *testing.T) {
	store := &customerStoreStub{find: func(string, string) (customer.Customer, error) {
		return customer.Customer{}, errors.New("postgres internal detail")
	}}
	response := httptest.NewRecorder()
	newLookupHandler(t, store).ServeHTTP(response, newLookupRequest(`{"phone":"+596696123456"}`, voiceTokenA))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "postgres") {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestCustomerLookupRejectsCrossTenantStoreResult(t *testing.T) {
	store := &customerStoreStub{find: func(string, string) (customer.Customer, error) {
		return customer.Customer{ID: "customer-b", TenantID: voiceTenantB, FirstName: "Private"}, nil
	}}
	response := httptest.NewRecorder()
	newLookupHandler(t, store).ServeHTTP(response, newLookupRequest(`{"phone":"+596696123456"}`, voiceTokenA))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "Private") {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}
