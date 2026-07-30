package planning

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/domain"
	"github.com/esrid/garage/internal/core/tenant"
)

const (
	handlerCustomerID    = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea1"
	handlerAppointmentID = "019c09ea-bca7-7a5d-98b6-3f3b3ed79ea3"
)

type schedulingStub struct {
	bookInput       appointment.BookInput
	rescheduleInput appointment.RescheduleInput
	cancelInput     appointment.CancelInput
	appointment     appointment.Appointment
	day             appointment.Day
	err             error
	tenantID        string
}

func (s *schedulingStub) AvailableSlots(context.Context, appointment.AvailabilityQuery) ([]appointment.Slot, error) {
	return nil, s.err
}

func (s *schedulingStub) Book(ctx context.Context, input appointment.BookInput) (appointment.Appointment, error) {
	s.tenantID, _ = tenant.IDFromContext(ctx)
	s.bookInput = input
	return s.appointment, s.err
}

func (s *schedulingStub) Reschedule(ctx context.Context, input appointment.RescheduleInput) (appointment.Appointment, error) {
	s.tenantID, _ = tenant.IDFromContext(ctx)
	s.rescheduleInput = input
	return s.appointment, s.err
}

func (s *schedulingStub) Cancel(ctx context.Context, input appointment.CancelInput) (appointment.Appointment, error) {
	s.tenantID, _ = tenant.IDFromContext(ctx)
	s.cancelInput = input
	return s.appointment, s.err
}

func (s *schedulingStub) Day(context.Context, time.Time) (appointment.Day, error) {
	return s.day, s.err
}

func (s *schedulingStub) ConfigureOpening(context.Context, appointment.ConfigureOpeningInput) (appointment.Opening, error) {
	return appointment.Opening{}, s.err
}

func newMutationHandler(stub *schedulingStub) *AppointmentMutations {
	return NewAppointmentMutations(appointment.NewService(stub, stub, stub, nil))
}

func appointmentRequest(t *testing.T, target string, form url.Values, withTenant bool) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if withTenant {
		request = request.WithContext(tenant.WithID(request.Context(), "tenant-from-context"))
	}
	return request
}

func validBookForm() url.Values {
	return url.Values{
		"customer_id":      {handlerCustomerID},
		"service_label":    {"Révision"},
		"start_at":         {"2030-01-02T08:00:00-04:00"},
		"duration_minutes": {"60"},
		"idempotency_key":  {"book-1"},
	}
}

func TestAppointmentBookUsesContextAndRedirectsToPersistedTenantDay(t *testing.T) {
	start := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	stub := &schedulingStub{
		appointment: appointment.Appointment{ID: handlerAppointmentID, Start: start},
		day:         appointment.Day{Date: time.Date(2030, 1, 2, 0, 0, 0, 0, time.FixedZone("Martinique", -4*60*60))},
	}
	form := validBookForm()
	form.Set("tenant_id", "attacker-controlled")
	response := httptest.NewRecorder()
	newMutationHandler(stub).Book(response, appointmentRequest(t, "/app/appointments", form, true))

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Location"); got != "/app/planning?day=2030-01-02" {
		t.Fatalf("Location = %q", got)
	}
	if stub.tenantID != "tenant-from-context" || stub.bookInput.CustomerID != handlerCustomerID {
		t.Fatalf("tenant=%q input=%#v", stub.tenantID, stub.bookInput)
	}
}

func TestAppointmentRescheduleAndCancelUsePathID(t *testing.T) {
	start := time.Date(2030, 1, 3, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		form url.Values
		call func(*AppointmentMutations, http.ResponseWriter, *http.Request)
	}{
		{
			name: "reschedule",
			form: url.Values{"start_at": {"2030-01-03T08:00:00-04:00"}, "duration_minutes": {"60"}, "idempotency_key": {"move-1"}},
			call: (*AppointmentMutations).Reschedule,
		},
		{
			name: "cancel",
			form: url.Values{"idempotency_key": {"cancel-1"}},
			call: (*AppointmentMutations).Cancel,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &schedulingStub{
				appointment: appointment.Appointment{ID: handlerAppointmentID, Start: start},
				day:         appointment.Day{Date: time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC)},
			}
			request := appointmentRequest(t, "/app/appointments/ignored", test.form, true)
			request.SetPathValue("id", handlerAppointmentID)
			response := httptest.NewRecorder()
			test.call(newMutationHandler(stub), response, request)
			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, body=%q", response.Code, response.Body.String())
			}
			if test.name == "reschedule" && stub.rescheduleInput.AppointmentID != handlerAppointmentID {
				t.Fatalf("reschedule ID = %q", stub.rescheduleInput.AppointmentID)
			}
			if test.name == "cancel" && stub.cancelInput.AppointmentID != handlerAppointmentID {
				t.Fatalf("cancel ID = %q", stub.cancelInput.AppointmentID)
			}
		})
	}
}

func TestAppointmentBookMapsContractErrors(t *testing.T) {
	tests := []struct {
		name         string
		withTenant   bool
		providerErr  error
		mutate       func(url.Values)
		want         int
		wantLocation string
	}{
		{"missing tenant", false, nil, nil, http.StatusUnauthorized, ""},
		{"invalid form", true, nil, func(form url.Values) { form.Set("duration_minutes", "20") }, http.StatusSeeOther, "/app/planning?error=invalid#planning-alert"},
		{"not found", true, &domain.NotFoundError{Entity: "customer"}, nil, http.StatusSeeOther, "/app/planning?error=not_found#planning-alert"},
		{"capacity", true, appointment.ErrSlotUnavailable, nil, http.StatusSeeOther, "/app/planning?error=conflict#planning-alert"},
		{"idempotency", true, appointment.ErrIdempotencyConflict, nil, http.StatusSeeOther, "/app/planning?error=conflict#planning-alert"},
		{"provider unavailable", true, errors.New("database down"), nil, http.StatusSeeOther, "/app/planning?error=unavailable#planning-alert"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := validBookForm()
			if test.mutate != nil {
				test.mutate(form)
			}
			stub := &schedulingStub{err: test.providerErr}
			response := httptest.NewRecorder()
			newMutationHandler(stub).Book(response, appointmentRequest(t, "/app/appointments", form, test.withTenant))
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%q", response.Code, test.want, response.Body.String())
			}
			if location := response.Header().Get("Location"); location != test.wantLocation {
				t.Fatalf("Location = %q, want %q", location, test.wantLocation)
			}
			if strings.Contains(response.Body.String(), "database down") {
				t.Fatal("provider details leaked in HTTP response")
			}
		})
	}
}

func TestAppointmentBookRejectsWrongContentTypeAndOversizedBody(t *testing.T) {
	t.Run("content type", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/app/appointments", strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		newMutationHandler(&schedulingStub{}).Book(response, request)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/app/planning?error=invalid#planning-alert" {
			t.Fatalf("status = %d location=%q", response.Code, response.Header().Get("Location"))
		}
	})
	t.Run("body limit", func(t *testing.T) {
		request := appointmentRequest(t, "/app/appointments", url.Values{"note": {strings.Repeat("x", maxAppointmentFormBytes)}}, true)
		response := httptest.NewRecorder()
		newMutationHandler(&schedulingStub{}).Book(response, request)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/app/planning?error=invalid#planning-alert" {
			t.Fatalf("status = %d location=%q", response.Code, response.Header().Get("Location"))
		}
	})
}

func TestAppointmentRescheduleAndCancelErrorsUsePlanningRedirect(t *testing.T) {
	for _, test := range []struct {
		name string
		form url.Values
		call func(*AppointmentMutations, http.ResponseWriter, *http.Request)
	}{
		{
			name: "reschedule",
			form: url.Values{"start_at": {"2030-01-03T08:00:00-04:00"}, "duration_minutes": {"60"}, "idempotency_key": {"move-conflict"}},
			call: (*AppointmentMutations).Reschedule,
		},
		{
			name: "cancel",
			form: url.Values{"idempotency_key": {"cancel-conflict"}},
			call: (*AppointmentMutations).Cancel,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := appointmentRequest(t, "/app/appointments/"+handlerAppointmentID, test.form, true)
			request.SetPathValue("id", handlerAppointmentID)
			response := httptest.NewRecorder()
			test.call(newMutationHandler(&schedulingStub{err: appointment.ErrSlotUnavailable}), response, request)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/app/planning?error=conflict#planning-alert" {
				t.Fatalf("status=%d location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
			}
		})
	}
}

// The row shows what the domain allows and nothing else: a vehicle in progress
// cannot be moved, but it can be finished, and a terminal one offers nothing.
//
// Rows are found by the person they are about, not by an id: a terminal row has
// no form at all, so its id appears nowhere in the markup - which is the point.
func rowFor(t *testing.T, body, customer string) string {
	t.Helper()
	rows := strings.Split(body, `<li class="item">`)
	for _, row := range rows {
		if strings.Contains(row, customer) {
			return row
		}
	}
	t.Fatalf("no row for %s", customer)
	return ""
}

func TestPlanningRowsOfferOnlyTheAllowedMoves(t *testing.T) {
	stub := fullPlanningStub()
	stub.appointments[0].Status = appointment.StatusInProgress

	body := getPlanning(t, newTestPlanning(stub).Page, "/app/planning").Body.String()

	inProgress := rowFor(t, body, "Marie Lubin")
	if !strings.Contains(inProgress, `value="done"`) {
		t.Error("an appointment in progress cannot be finished")
	}
	for _, forbidden := range []string{"/reschedule", "/cancel", `value="confirmed"`} {
		if strings.Contains(inProgress, forbidden) {
			t.Errorf("an appointment in progress still offers %q", forbidden)
		}
	}

	terminal := rowFor(t, body, "Garage Morne-Rouge")
	for _, forbidden := range []string{"/status", "/reschedule", "/cancel"} {
		if strings.Contains(terminal, forbidden) {
			t.Errorf("a cancelled appointment still offers %q", forbidden)
		}
	}
}

// A pending row is confirmed from the desk, which is the first move of the table.
func TestPlanningPendingRowCanBeConfirmed(t *testing.T) {
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String()
	row := rowFor(t, body, "Jean-Claude Sainte-Rose")

	if !strings.Contains(row, `value="confirmed"`) || !strings.Contains(row, "Confirmer") {
		t.Error("a pending appointment cannot be confirmed from the planning")
	}
}
