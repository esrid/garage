package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/core/tenant"
	"github.com/esrid/garage/internal/web/views"
)

// The compositions that fill the day view: appointments, the persisted calls,
// and the shell they render into. They live with the feature they belong to.

func TestTodayWithCallsProviderFillsDashboardWithoutInventingSubject(t *testing.T) {
	day := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	base := &stubProvider{data: views.Today{Day: day, Calls: []views.Call{{ID: "fixture"}}}}
	calls := callHistoryReaderStub{history: views.CallHistory{Calls: []views.CallSummary{{
		ID: "call-1", At: day.Add(9 * time.Hour), Duration: 2 * time.Minute,
		Outcome: "transferred", Summary: "Résumé assistant non vérifié",
	}}}}
	result, err := NewTodayWithCallsProvider(base, calls).Today(context.Background(), day)
	if err != nil {
		t.Fatalf("Today() error = %v", err)
	}
	if len(result.Calls) != 1 || result.Calls[0].ID != "call-1" || !result.Calls[0].Transferred || result.Calls[0].Subject != "" {
		t.Fatalf("dashboard calls = %#v", result.Calls)
	}
}
func TestTodayWithCallsProviderPropagatesReadFailure(t *testing.T) {
	want := errors.New("database down")
	_, err := NewTodayWithCallsProvider(&stubProvider{}, callHistoryReaderStub{err: want}).Today(context.Background(), time.Now())
	if !errors.Is(err, want) {
		t.Fatalf("Today() error = %v, want %v", err, want)
	}
}
func TestAppointmentTodayProviderMapsOnlyPersistedAppointments(t *testing.T) {
	location := time.FixedZone("Martinique", -4*60*60)
	day := time.Date(2030, 1, 2, 0, 0, 0, 0, location)
	startUTC := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	stub := dayReaderStub{day: appointment.Day{
		Date: day,
		Appointments: []appointment.DayEntry{{
			Appointment: appointment.Appointment{
				ID: "019c09ea-bca7-7a5d-98b6-3f3b3ed79ec1", Start: startUTC, End: startUTC.Add(time.Hour),
				ServiceLabel: "Révision", Status: appointment.StatusConfirmed,
			},
			CustomerName: "Ana Césaire", VehicleLabel: "Renault Clio", Plate: "",
		}},
	}}
	result, err := NewAppointmentTodayProvider(stub).Today(tenant.WithID(context.Background(), "tenant-1"), day)
	if err != nil {
		t.Fatalf("Today() error = %v", err)
	}
	if len(result.Appointments) != 1 || result.Appointments[0].CustomerName != "Ana Césaire" || result.Appointments[0].Plate != "" {
		t.Fatalf("Today() = %#v", result)
	}
	mapped := result.Appointments[0]
	if mapped.Start.Location() != location || mapped.Start.Hour() != 8 || !mapped.Start.Equal(startUTC) || mapped.End.Hour() != 9 {
		t.Fatalf("mapped appointment times = %v–%v, want 08:00–09:00 Martinique preserving instant", mapped.Start, mapped.End)
	}
	if len(result.Calls) != 0 || len(result.Tasks) != 0 {
		t.Fatalf("Today() invented calls or tasks: %#v", result)
	}
}

// Signing out has to be a POST: F09 revokes the server session there, and a GET
// logout is firable by any third-party image tag.
func TestAppShellSignsOutWithAForm(t *testing.T) {
	body := get(t, newTestDashboard(&stubProvider{data: dashboardPreviewData()}).Page, "/app").Body.String()

	if !strings.Contains(body, `method="post" action="/auth/logout"`) {
		t.Error("the app shell has no logout form")
	}
	if strings.Contains(body, `href="/auth/logout"`) {
		t.Error("logout must not be a link")
	}
}

// callHistoryReaderStub stands in for the call history feature: the dashboard
// composes it, so the port is declared here and the double lives here too.
type callHistoryReaderStub struct {
	history views.CallHistory
	err     error
}

func (s callHistoryReaderStub) Calls(context.Context, time.Time) (views.CallHistory, error) {
	return s.history, s.err
}

// dayReaderStub is all the appointment composition needs: the persisted day.
type dayReaderStub struct {
	day appointment.Day
	err error
}

func (s dayReaderStub) Day(context.Context, time.Time) (appointment.Day, error) {
	return s.day, s.err
}

// dashboardPreviewData fills the three dashboard panels. Calls and tasks have no
// backend yet, so this is the only way to look at those panels rendered.
func dashboardPreviewData() views.Today {
	return views.Today{
		Day: previewAt(9, 30),
		Calls: []views.Call{
			{
				ID: "call-1", At: previewAt(8, 12), Duration: 4*time.Minute + 20*time.Second,
				CustomerName: "Marie Lubin", Phone: "0596000001",
				Subject: "Vidange + révision", Outcome: "booked",
			},
			{
				ID: "call-2", At: previewAt(9, 3), Duration: 2 * time.Minute,
				Phone: "0696000002", Subject: "Devis embrayage", Outcome: "quote",
			},
		},
		Appointments: []views.Appointment{{
			ID: "rdv-1", Start: previewAt(9, 0), End: previewAt(10, 0),
			CustomerName: "Marie Lubin", Vehicle: "Clio IV", Plate: "AB-123-CD",
			Service: "Vidange", Status: "confirmed",
		}},
		Tasks: []views.Task{{
			ID: "task-1", CreatedAt: previewAt(9, 5), Kind: "quote",
			Phone: "0696000002", Note: "Rappeler pour le devis embrayage",
		}},
	}
}

// previewAt is the fixture clock for the preview data.
func previewAt(hour, minute int) time.Time {
	return time.Date(2026, 7, 30, hour, minute, 0, 0, time.FixedZone("AST", -4*60*60))
}
