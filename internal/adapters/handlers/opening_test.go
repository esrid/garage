package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/tenant"
)

// planningDayReaderStub answers like the PostgreSQL day reader: the requested
// instant resolved to midnight in the workshop's timezone.
type planningDayReaderStub struct{}

func (planningDayReaderStub) Day(_ context.Context, day time.Time) (appointment.Day, error) {
	local := day.In(martinique)
	return appointment.Day{
		Date:     time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, martinique),
		Timezone: "America/Martinique",
	}, nil
}

type openingConfigurerStub struct {
	input appointment.ConfigureOpeningInput
	err   error
	calls int
}

func (s *openingConfigurerStub) ConfigureOpening(_ context.Context, input appointment.ConfigureOpeningInput) (appointment.Opening, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return appointment.Opening{}, s.err
	}
	return appointment.Opening{ID: "op-1", Start: input.Start, End: input.End, Capacity: input.Capacity}, nil
}

func postOpening(t *testing.T, service *appointment.Service, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/app/openings", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// The session middleware is what puts the tenant in the context in production;
	// these tests stand in for it rather than letting the service refuse the call.
	request = request.WithContext(tenant.WithID(request.Context(), "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1"))
	response := httptest.NewRecorder()
	NewOpeningMutations(service).Configure(response, request)
	return response
}

// The garage types wall-clock hours; the instants must be built in the workshop's
// timezone. 08:00 in Martinique is 12:00 UTC, and getting that wrong moves the
// whole day by four hours.
func TestOpeningTimesAreBuiltInTheWorkshopTimezone(t *testing.T) {
	configurer := &openingConfigurerStub{}
	service := appointment.NewService(nil, &planningDayReaderStub{}, configurer)

	response := postOpening(t, service, url.Values{
		"day": {"2026-07-31"}, "starts_at": {"08:00"}, "ends_at": {"17:00"}, "capacity": {"2"},
	})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/app/planning?day=2026-07-31" {
		t.Errorf("Location = %q", got)
	}
	if got := configurer.input.Start.Format(time.RFC3339); got != "2026-07-31T08:00:00-04:00" {
		t.Errorf("start = %s, want the workshop's 08:00", got)
	}
	if got := configurer.input.End.Format(time.RFC3339); got != "2026-07-31T17:00:00-04:00" {
		t.Errorf("end = %s, want the workshop's 17:00", got)
	}
	if configurer.input.Capacity != 2 {
		t.Errorf("capacity = %d, want 2", configurer.input.Capacity)
	}
}

// A malformed field must not reach the service, and the operator must land back
// on the planning with a readable reason rather than on a bare error page.
func TestOpeningRejectsUnusableInput(t *testing.T) {
	for name, form := range map[string]url.Values{
		"no time":        {"day": {"2026-07-31"}, "starts_at": {""}, "ends_at": {"17:00"}, "capacity": {"1"}},
		"not a clock":    {"day": {"2026-07-31"}, "starts_at": {"huit heures"}, "ends_at": {"17:00"}, "capacity": {"1"}},
		"no capacity":    {"day": {"2026-07-31"}, "starts_at": {"08:00"}, "ends_at": {"17:00"}, "capacity": {"beaucoup"}},
		"unreadable day": {"day": {"31/07/2026"}, "starts_at": {"08:00"}, "ends_at": {"17:00"}, "capacity": {"1"}},
	} {
		t.Run(name, func(t *testing.T) {
			configurer := &openingConfigurerStub{}
			response := postOpening(t, appointment.NewService(nil, &planningDayReaderStub{}, configurer), form)

			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", response.Code)
			}
			if got := response.Header().Get("Location"); !strings.Contains(got, "error=invalid") {
				t.Errorf("Location = %q, want an invalid error code", got)
			}
			if configurer.calls != 0 {
				t.Error("an unusable form reached the service")
			}
		})
	}
}
