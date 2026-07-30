package voicetools

import (
	"context"
	"errors"
	"github.com/esrid/garage/internal/adapters/voice"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	voiceCustomerA    = "019c09ea-bca7-7a5d-98b6-3f3b3ed79eb1"
	voiceAppointmentA = "019c09ea-bca7-7a5d-98b6-3f3b3ed79eb2"
)

type schedulingProviderStub struct {
	available func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error)
	book      func(context.Context, appointment.BookInput) (appointment.Appointment, error)
	day       func(context.Context, time.Time) (appointment.Day, error)
}

func (s *schedulingProviderStub) AvailableSlots(ctx context.Context, query appointment.AvailabilityQuery) ([]appointment.Slot, error) {
	return s.available(ctx, query)
}

func (s *schedulingProviderStub) Book(ctx context.Context, input appointment.BookInput) (appointment.Appointment, error) {
	return s.book(ctx, input)
}

func (*schedulingProviderStub) Reschedule(context.Context, appointment.RescheduleInput) (appointment.Appointment, error) {
	return appointment.Appointment{}, errors.New("not used")
}

func (*schedulingProviderStub) Cancel(context.Context, appointment.CancelInput) (appointment.Appointment, error) {
	return appointment.Appointment{}, errors.New("not used")
}

func (s *schedulingProviderStub) Day(ctx context.Context, instant time.Time) (appointment.Day, error) {
	if s.day != nil {
		return s.day(ctx, instant)
	}
	location := time.FixedZone("America/Martinique", -4*60*60)
	local := instant.In(location)
	return appointment.Day{
		Date:     time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location),
		Timezone: "America/Martinique",
	}, nil
}

func newAppointmentTools(t *testing.T, provider *schedulingProviderStub) *AppointmentTools {
	t.Helper()
	authenticator, err := voice.NewTokenAuthenticator(voiceTenantA + ":" + voiceTokenA + "," + voiceTenantB + ":" + voiceTokenB)
	if err != nil {
		t.Fatalf("voice.NewTokenAuthenticator() error = %v", err)
	}
	return NewAppointmentTools(appointment.NewService(provider, provider, nil, nil), authenticator)
}

func newAppointmentToolRequest(path, body, token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func TestAppointmentAvailabilityReturnsOnlyProviderSlotsForAuthenticatedTenant(t *testing.T) {
	start := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	provider := &schedulingProviderStub{
		available: func(ctx context.Context, query appointment.AvailabilityQuery) ([]appointment.Slot, error) {
			tenantID, err := tenant.IDFromContext(ctx)
			if err != nil || tenantID != voiceTenantA {
				t.Fatalf("tenant = %q, %v", tenantID, err)
			}
			if query.Day.Format(time.RFC3339) != "2030-01-02T12:00:00-04:00" || query.Duration != time.Hour {
				t.Fatalf("query = %#v", query)
			}
			return []appointment.Slot{{Start: start, End: start.Add(time.Hour)}}, nil
		},
		book: func(context.Context, appointment.BookInput) (appointment.Appointment, error) {
			t.Fatal("Book must not be called")
			return appointment.Appointment{}, nil
		},
	}
	response := httptest.NewRecorder()
	newAppointmentTools(t, provider).Availability(response, newAppointmentToolRequest(
		"/voice/tools/appointment-availability",
		`{"day":"2030-01-02T12:00:00-04:00","duration_minutes":60}`,
		voiceTokenA,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	want := `{"slots":[{"start_at":"2030-01-02T08:00:00-04:00","end_at":"2030-01-02T09:00:00-04:00"}]}`
	if strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("body=%q, want %q", response.Body.String(), want)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("availability response is cacheable")
	}
}

func TestAppointmentAvailabilityReturnsEmptyArray(t *testing.T) {
	provider := &schedulingProviderStub{
		available: func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) { return nil, nil },
		book: func(context.Context, appointment.BookInput) (appointment.Appointment, error) {
			return appointment.Appointment{}, nil
		},
	}
	response := httptest.NewRecorder()
	newAppointmentTools(t, provider).Availability(response, newAppointmentToolRequest(
		"/voice/tools/appointment-availability",
		`{"day":"2030-01-02T12:00:00-04:00","duration_minutes":60}`,
		voiceTokenA,
	))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"slots":[]}` {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestAppointmentAvailabilityHidesProviderFailure(t *testing.T) {
	provider := &schedulingProviderStub{
		available: func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) {
			return nil, errors.New("postgres secret")
		},
		book: func(context.Context, appointment.BookInput) (appointment.Appointment, error) {
			return appointment.Appointment{}, nil
		},
	}
	response := httptest.NewRecorder()
	newAppointmentTools(t, provider).Availability(response, newAppointmentToolRequest(
		"/voice/tools/appointment-availability",
		`{"day":"2030-01-02T12:00:00-04:00","duration_minutes":60}`,
		voiceTokenA,
	))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "postgres") {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestAppointmentBookConfirmsOnlyCommittedTenantResultAndUsesStableKey(t *testing.T) {
	start := time.Date(2030, 1, 2, 8, 0, 0, 0, time.FixedZone("AST", -4*60*60))
	var keys []string
	provider := &schedulingProviderStub{
		available: func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) { return nil, nil },
		book: func(ctx context.Context, input appointment.BookInput) (appointment.Appointment, error) {
			tenantID, err := tenant.IDFromContext(ctx)
			if err != nil || tenantID != voiceTenantA {
				t.Fatalf("tenant = %q, %v", tenantID, err)
			}
			if input.CustomerID != voiceCustomerA || input.ServiceLabel != "Révision" || input.Start.Format(time.RFC3339) != start.Format(time.RFC3339) || input.Duration != time.Hour {
				t.Fatalf("input = %#v", input)
			}
			keys = append(keys, input.IdempotencyKey)
			return appointment.Appointment{
				ID: voiceAppointmentA, TenantID: voiceTenantA, CustomerID: input.CustomerID,
				Start: input.Start.UTC(), End: input.Start.Add(input.Duration).UTC(), Status: appointment.StatusConfirmed,
			}, nil
		},
	}
	body := `{"conversation_id":"conv_123","customer_id":"` + voiceCustomerA + `","vehicle_id":"","service_label":" Révision ","start_at":"2030-01-02T08:00:00-04:00","duration_minutes":60,"note":""}`
	handler := newAppointmentTools(t, provider)
	for range 2 {
		response := httptest.NewRecorder()
		handler.Book(response, newAppointmentToolRequest("/voice/tools/appointment-book", body, voiceTokenA))
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
		for _, want := range []string{`"confirmed":true`, `"id":"` + voiceAppointmentA + `"`, `"status":"confirmed"`} {
			if !strings.Contains(response.Body.String(), want) {
				t.Errorf("body %q missing %q", response.Body.String(), want)
			}
		}
		for _, want := range []string{`"start_at":"2030-01-02T08:00:00-04:00"`, `"end_at":"2030-01-02T09:00:00-04:00"`} {
			if !strings.Contains(response.Body.String(), want) {
				t.Errorf("body %q missing tenant-local time %q", response.Body.String(), want)
			}
		}
		for _, forbidden := range []string{voiceTenantA, voiceCustomerA, "conv_123"} {
			if strings.Contains(response.Body.String(), forbidden) {
				t.Errorf("body leaked %q: %s", forbidden, response.Body.String())
			}
		}
	}
	if len(keys) != 2 || keys[0] != keys[1] || !strings.HasPrefix(keys[0], "voice-book-") || len(keys[0]) > 200 {
		t.Fatalf("idempotency keys = %#v", keys)
	}
}

func TestAppointmentBookDifferentOperationGetsDifferentKey(t *testing.T) {
	first := bookingRequest{ConversationID: "conv_123", CustomerID: voiceCustomerA, ServiceLabel: "Révision", Note: "A"}
	second := first
	second.Note = "B"
	start := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	if appointmentToolIdempotencyKey(first, start, time.Hour) == appointmentToolIdempotencyKey(second, start, time.Hour) {
		t.Fatal("materially different bookings share an idempotency key")
	}
}

func TestAppointmentBookDoesNotWriteWhenTenantTimezoneIsUnavailable(t *testing.T) {
	provider := &schedulingProviderStub{
		available: func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) { return nil, nil },
		book: func(context.Context, appointment.BookInput) (appointment.Appointment, error) {
			t.Fatal("Book must not be called before the tenant timezone is validated")
			return appointment.Appointment{}, nil
		},
		day: func(context.Context, time.Time) (appointment.Day, error) {
			return appointment.Day{}, errors.New("timezone store unavailable")
		},
	}
	body := `{"conversation_id":"conv_123","customer_id":"` + voiceCustomerA + `","vehicle_id":"","service_label":"Révision","start_at":"2030-01-02T08:00:00-04:00","duration_minutes":60,"note":""}`
	response := httptest.NewRecorder()
	newAppointmentTools(t, provider).Book(response, newAppointmentToolRequest("/voice/tools/appointment-book", body, voiceTokenA))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"confirmed":false`) {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestAppointmentToolsRejectUnauthorizedAndInvalidRequests(t *testing.T) {
	provider := &schedulingProviderStub{
		available: func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) {
			t.Fatal("AvailableSlots must not be called")
			return nil, nil
		},
		book: func(context.Context, appointment.BookInput) (appointment.Appointment, error) {
			t.Fatal("Book must not be called")
			return appointment.Appointment{}, nil
		},
	}
	handler := newAppointmentTools(t, provider)
	tests := []struct {
		name         string
		availability bool
		body         string
		token        string
		contentType  string
		want         int
	}{
		{"availability auth", true, `{"day":"2030-01-02T12:00:00-04:00","duration_minutes":60}`, "", "application/json", http.StatusUnauthorized},
		{"availability day", true, `{"day":"2030-01-02","duration_minutes":60}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"availability duration", true, `{"day":"2030-01-02T12:00:00-04:00","duration_minutes":17}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"booking auth", false, `{}`, "", "application/json", http.StatusUnauthorized},
		{"booking content type", false, `{}`, voiceTokenA, "text/plain", http.StatusUnprocessableEntity},
		{"booking unknown tenant", false, `{"conversation_id":"conv","tenant_id":"` + voiceTenantB + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"booking conversation", false, `{"conversation_id":"","customer_id":"` + voiceCustomerA + `","service_label":"Révision","start_at":"2030-01-02T08:00:00-04:00","duration_minutes":60}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
		{"booking oversized", false, `{"conversation_id":"` + strings.Repeat("x", maxAppointmentToolBodyBytes) + `"}`, voiceTokenA, "application/json", http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newAppointmentToolRequest("/voice/tools/test", test.body, test.token)
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			if test.availability {
				handler.Availability(response, request)
			} else {
				handler.Book(response, request)
			}
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%q", response.Code, test.want, response.Body.String())
			}
			if test.want == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("401 response is missing the Bearer challenge")
			}
			if strings.Contains(response.Body.String(), voiceTenantA) || strings.Contains(response.Body.String(), voiceTokenA) {
				t.Fatal("error response leaked tenant or credential")
			}
		})
	}
}

func TestAppointmentBookMapsFailuresWithoutConfirming(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		result appointment.Appointment
		want   int
	}{
		{"validation", &domain.ValidationError{Entity: "appointment"}, appointment.Appointment{}, http.StatusUnprocessableEntity},
		{"not found", &domain.NotFoundError{Entity: "customer"}, appointment.Appointment{}, http.StatusNotFound},
		{"slot unavailable", appointment.ErrSlotUnavailable, appointment.Appointment{}, http.StatusConflict},
		{"idempotency", appointment.ErrIdempotencyConflict, appointment.Appointment{}, http.StatusConflict},
		{"provider", errors.New("postgres secret"), appointment.Appointment{}, http.StatusServiceUnavailable},
		{"not confirmed", nil, appointment.Appointment{ID: voiceAppointmentA, TenantID: voiceTenantA, Start: time.Now(), End: time.Now().Add(time.Hour), Status: appointment.StatusPending}, http.StatusServiceUnavailable},
		{"cross tenant", nil, appointment.Appointment{ID: voiceAppointmentA, TenantID: voiceTenantB, Start: time.Now(), End: time.Now().Add(time.Hour), Status: appointment.StatusConfirmed}, http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &schedulingProviderStub{
				available: func(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) { return nil, nil },
				book: func(context.Context, appointment.BookInput) (appointment.Appointment, error) {
					return test.result, test.err
				},
			}
			body := `{"conversation_id":"conv_123","customer_id":"` + voiceCustomerA + `","vehicle_id":"","service_label":"Révision","start_at":"2030-01-02T08:00:00-04:00","duration_minutes":60,"note":""}`
			response := httptest.NewRecorder()
			newAppointmentTools(t, provider).Book(response, newAppointmentToolRequest("/voice/tools/appointment-book", body, voiceTokenA))
			if response.Code != test.want || !strings.Contains(response.Body.String(), `"confirmed":false`) {
				t.Fatalf("status=%d body=%q, want status=%d and unconfirmed", response.Code, response.Body.String(), test.want)
			}
			if strings.Contains(response.Body.String(), "postgres") || strings.Contains(response.Body.String(), voiceTenantB) {
				t.Fatalf("failure leaked internals: %q", response.Body.String())
			}
		})
	}
}
