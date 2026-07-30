package appointment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	testCustomerID    = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1"
	testVehicleID     = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea2"
	testAppointmentID = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea3"
)

type providerStub struct {
	bookInput       BookInput
	rescheduleInput RescheduleInput
	cancelInput     CancelInput
	called          bool
}

func (s *providerStub) AvailableSlots(context.Context, AvailabilityQuery) ([]Slot, error) {
	s.called = true
	return nil, nil
}

func (s *providerStub) Book(_ context.Context, input BookInput) (Appointment, error) {
	s.called = true
	s.bookInput = input
	return Appointment{ID: testAppointmentID}, nil
}

func (s *providerStub) Reschedule(_ context.Context, input RescheduleInput) (Appointment, error) {
	s.called = true
	s.rescheduleInput = input
	return Appointment{ID: testAppointmentID}, nil
}

func (s *providerStub) Cancel(_ context.Context, input CancelInput) (Appointment, error) {
	s.called = true
	s.cancelInput = input
	return Appointment{ID: testAppointmentID}, nil
}

func (s *providerStub) Day(context.Context, time.Time) (Day, error) {
	s.called = true
	return Day{}, nil
}

func (s *providerStub) ConfigureOpening(context.Context, ConfigureOpeningInput) (Opening, error) {
	s.called = true
	return Opening{}, nil
}

func TestServiceBookTrimsInputAndUsesTenantContext(t *testing.T) {
	provider := &providerStub{}
	service := NewService(provider, provider, provider, nil)
	start := time.Date(2030, 1, 2, 8, 0, 0, 0, time.UTC)

	_, err := service.Book(tenant.WithID(context.Background(), "tenant-1"), BookInput{
		CustomerID: " " + testCustomerID + " ", VehicleID: " " + testVehicleID + " ",
		ServiceLabel: " Révision ", Note: " Prévoir filtre ", Start: start,
		Duration: time.Hour, IdempotencyKey: " request-1 ",
	})
	if err != nil {
		t.Fatalf("Book() error = %v", err)
	}
	if provider.bookInput.CustomerID != testCustomerID || provider.bookInput.VehicleID != testVehicleID ||
		provider.bookInput.ServiceLabel != "Révision" || provider.bookInput.Note != "Prévoir filtre" ||
		provider.bookInput.IdempotencyKey != "request-1" {
		t.Fatalf("provider input = %#v", provider.bookInput)
	}
}

func TestServiceRejectsMissingTenantBeforeProvider(t *testing.T) {
	provider := &providerStub{}
	service := NewService(provider, provider, provider, nil)
	_, err := service.Cancel(context.Background(), CancelInput{AppointmentID: testAppointmentID, IdempotencyKey: "cancel-1"})
	var unauthorized *domain.UnauthorizedError
	if !errors.As(err, &unauthorized) || provider.called {
		t.Fatalf("Cancel() error=%v provider.called=%t", err, provider.called)
	}
}

func TestServiceRejectsInvalidWriteInputs(t *testing.T) {
	ctx := tenant.WithID(context.Background(), "tenant-1")
	start := time.Date(2030, 1, 2, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		input BookInput
	}{
		{"customer UUID", BookInput{CustomerID: "not-a-uuid", ServiceLabel: "Révision", Start: start, Duration: time.Hour, IdempotencyKey: "key"}},
		{"vehicle UUID", BookInput{CustomerID: testCustomerID, VehicleID: "bad", ServiceLabel: "Révision", Start: start, Duration: time.Hour, IdempotencyKey: "key"}},
		{"duration increment", BookInput{CustomerID: testCustomerID, ServiceLabel: "Révision", Start: start, Duration: 20 * time.Minute, IdempotencyKey: "key"}},
		{"service length", BookInput{CustomerID: testCustomerID, ServiceLabel: strings.Repeat("é", 201), Start: start, Duration: time.Hour, IdempotencyKey: "key"}},
		{"note length", BookInput{CustomerID: testCustomerID, ServiceLabel: "Révision", Note: strings.Repeat("à", 2001), Start: start, Duration: time.Hour, IdempotencyKey: "key"}},
		{"idempotency length", BookInput{CustomerID: testCustomerID, ServiceLabel: "Révision", Start: start, Duration: time.Hour, IdempotencyKey: strings.Repeat("é", 201)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &providerStub{}
			_, err := NewService(provider, provider, provider, nil).Book(ctx, test.input)
			var validation *domain.ValidationError
			if !errors.As(err, &validation) || provider.called {
				t.Fatalf("Book() error=%v provider.called=%t", err, provider.called)
			}
		})
	}
}

func TestServiceAcceptsUnicodeAtCharacterLimit(t *testing.T) {
	provider := &providerStub{}
	service := NewService(provider, provider, provider, nil)
	_, err := service.Book(tenant.WithID(context.Background(), "tenant-1"), BookInput{
		CustomerID: testCustomerID, ServiceLabel: strings.Repeat("é", 200),
		Start: time.Now(), Duration: time.Hour, IdempotencyKey: "unicode-limit",
	})
	if err != nil || !provider.called {
		t.Fatalf("Book() error=%v provider.called=%t", err, provider.called)
	}
}

// The transition table is the contract frozen in docs/contracts/F02A-planning.md.
// It lives in the domain so a desk button and a voice tool cannot disagree about
// what is allowed.
func TestTransitionTableMatchesTheFrozenContract(t *testing.T) {
	allowed := map[Status][]Status{
		StatusPending:    {StatusConfirmed, StatusCancelled},
		StatusConfirmed:  {StatusInProgress, StatusCancelled, StatusNoShow},
		StatusInProgress: {StatusDone},
		StatusDone:       nil,
		StatusCancelled:  nil,
		StatusNoShow:     nil,
	}
	every := []Status{StatusPending, StatusConfirmed, StatusInProgress, StatusDone, StatusCancelled, StatusNoShow}

	for from, wanted := range allowed {
		for _, to := range every {
			want := false
			for _, candidate := range wanted {
				if candidate == to {
					want = true
				}
			}
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", from, to, got, want)
			}
		}
		if got := len(NextStatuses(from)); got != len(wanted) {
			t.Errorf("NextStatuses(%q) has %d entries, want %d", from, got, len(wanted))
		}
	}
}

type statusUpdaterStub struct {
	input UpdateStatusInput
	calls int
}

func (s *statusUpdaterStub) UpdateAppointmentStatus(_ context.Context, input UpdateStatusInput) (Appointment, error) {
	s.calls++
	s.input = input
	return Appointment{ID: input.AppointmentID, Status: input.Status}, nil
}

func TestUpdateStatusRefusesWhatIsNotAStatus(t *testing.T) {
	updater := &statusUpdaterStub{}
	service := NewService(nil, nil, nil, updater)
	ctx := tenant.WithID(context.Background(), "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1")

	if _, err := service.UpdateStatus(ctx, UpdateStatusInput{AppointmentID: "a", Status: "terminé"}); err == nil {
		t.Error("a status outside the closed set was accepted")
	}
	if _, err := service.UpdateStatus(ctx, UpdateStatusInput{Status: StatusDone}); err == nil {
		t.Error("a move without an appointment was accepted")
	}
	if updater.calls != 0 {
		t.Error("an unusable move reached the store")
	}

	if _, err := service.UpdateStatus(context.Background(), UpdateStatusInput{AppointmentID: "a", Status: StatusDone}); err == nil {
		t.Error("a move without a tenant was accepted")
	}
}
