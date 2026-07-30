package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/web/views"
)

var testDay = time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC)

type stubProvider struct {
	data views.Today
	err  error
	// seen records the day the handler asked for.
	seen time.Time
}

func (s *stubProvider) Today(_ context.Context, day time.Time) (views.Today, error) {
	s.seen = day
	return s.data, s.err
}

// newTestDashboard pins the clock: the rendered day is then a function of input
// only, so these tests do not drift with the wall clock.
func newTestDashboard(provider TodayProvider) *Dashboard {
	d := NewDashboard(provider)
	d.now = func() time.Time { return testDay }
	return d
}

func get(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	h(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	return response
}

func TestPageRendersTheDay(t *testing.T) {
	provider := &stubProvider{data: views.Today{
		Day: testDay,
		Calls: []views.Call{{
			At: testDay, Duration: 4*time.Minute + 20*time.Second,
			CustomerName: "Marie Lubin", Subject: "Vidange", Outcome: "booked",
		}},
		Appointments: []views.Appointment{{
			Start: testDay, CustomerName: "Marie Lubin",
			Vehicle: "Clio IV", Plate: "AB-123-CD", Service: "Vidange", Status: "confirmed",
		}},
	}}

	body := get(t, newTestDashboard(provider).Page, "/app").Body.String()

	for _, want := range []string{
		"<!doctype html>",       // full page, not a fragment
		"jeudi 30 juillet 2026", // the day, in French
		"Marie Lubin",
		"RDV pris",      // outcome label, not the raw value
		"badge-success", // tone derived from the status
		"4 min 20 s",
		"AB-123-CD",
		`hx-get="/app/today"`, // refresh is enhanced, not JS-only
		`href="/app"`,         // and still works without htmx
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}

	if !provider.seen.Equal(testDay) {
		t.Errorf("provider asked for %v, want %v", provider.seen, testDay)
	}
}

func TestPageIsNotCacheable(t *testing.T) {
	response := get(t, newTestDashboard(&stubProvider{}).Page, "/app")
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", got)
	}
}

// The fragment and the page must render the same panels: that is the whole
// reason the page embeds the fragment component.
func TestFragmentIsPanelsOnly(t *testing.T) {
	provider := &stubProvider{data: views.Today{Day: testDay}}
	body := get(t, newTestDashboard(provider).Fragment, "/app/today").Body.String()

	if strings.Contains(body, "<!doctype html>") || strings.Contains(body, "<body") {
		t.Error("the fragment returned a full page")
	}
	if !strings.Contains(body, `id="today"`) {
		t.Error("the fragment is missing the htmx swap target")
	}
}

// The classic htmx breakage: someone renames the fragment's root id and the
// refresh silently stops swapping. The page's hx-target and the fragment's root
// id must agree, in both the normal and the degraded branch.
func TestRefreshTargetMatchesFragmentRoot(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider *stubProvider
	}{
		{"normal", &stubProvider{data: views.Today{Day: testDay}}},
		{"degraded", &stubProvider{err: errors.New("down")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dashboard := newTestDashboard(test.provider)
			page := get(t, dashboard.Page, "/app").Body.String()
			fragment := get(t, dashboard.Fragment, "/app/today").Body.String()

			if !strings.Contains(page, `hx-target="#today"`) || !strings.Contains(page, `hx-get="/app/today"`) {
				t.Fatal("the page does not point htmx at the fragment route")
			}
			if !strings.Contains(fragment, `id="today"`) {
				t.Error(`the fragment root is not id="today", so the swap would not apply`)
			}
		})
	}
}

// A failing provider must degrade the page, never blank it and never 500.
func TestProviderErrorDegradesThePage(t *testing.T) {
	provider := &stubProvider{err: errors.New("database is down")}
	body := get(t, newTestDashboard(provider).Page, "/app").Body.String()

	if !strings.Contains(body, "momentanément indisponibles") {
		t.Error("the degraded notice is missing")
	}
	if !strings.Contains(body, "jeudi 30 juillet 2026") {
		t.Error("the page lost its shell when the provider failed")
	}
}

func TestEmptyDayShowsEmptyStates(t *testing.T) {
	body := get(t, newTestDashboard(&stubProvider{data: views.Today{Day: testDay}}).Page, "/app").Body.String()

	if count := strings.Count(body, "Rien à afficher aujourd'hui."); count != 3 {
		t.Errorf("got %d empty states, want 3 (calls, appointments, tasks)", count)
	}
}

// An unrecognised status is an integration bug: show it raw with a neutral
// badge rather than hiding the row or inventing a label.
func TestUnknownStatusIsShownRawAndNeutral(t *testing.T) {
	provider := &stubProvider{data: views.Today{
		Day: testDay,
		Appointments: []views.Appointment{{
			Start: testDay, CustomerName: "Marie Lubin", Status: "waiting_for_parts",
		}},
	}}

	body := get(t, newTestDashboard(provider).Page, "/app").Body.String()

	if !strings.Contains(body, "waiting_for_parts") {
		t.Error("the unknown status was hidden")
	}
	if !strings.Contains(body, "Marie Lubin") {
		t.Error("the row was dropped because of the unknown status")
	}
	for _, tone := range []string{"badge-success", "badge-warning", "badge-danger", "badge-info"} {
		if strings.Contains(body, tone) {
			t.Errorf("unknown status was given the %s tone", tone)
		}
	}
}

// Missing data is shown as an em dash, never filled in with a guess (PRD 7.1).
func TestMissingDataRendersAsDash(t *testing.T) {
	provider := &stubProvider{data: views.Today{
		Day: testDay,
		Appointments: []views.Appointment{{
			Start: testDay, CustomerName: "Marie Lubin", Service: "Diagnostic", Status: "pending",
		}},
		Calls: []views.Call{{At: testDay, Phone: "0596000009", Outcome: "info"}},
	}}

	body := get(t, newTestDashboard(provider).Page, "/app").Body.String()

	if strings.Contains(body, "item-plate") {
		t.Error("an absent plate was rendered as a plate")
	}
	if !strings.Contains(body, "—") {
		t.Error("missing values are not rendered as an em dash")
	}
	// No name, but a number: show the number rather than "unknown".
	if !strings.Contains(body, "0596000009") {
		t.Error("the caller number is missing when the name is unknown")
	}
}

// A row titled "—" while we hold the caller's number is useless at the desk.
// Calls and tasks both fall back to the number.
func TestRowsFallBackToThePhoneNumber(t *testing.T) {
	provider := &stubProvider{data: views.Today{
		Day:   testDay,
		Calls: []views.Call{{At: testDay, Phone: "0596111111", Outcome: "info"}},
		Tasks: []views.Task{{
			CreatedAt: testDay, Kind: "callback",
			Phone: "0696222222", Note: "Rappeler pour le devis",
		}},
	}}

	body := get(t, newTestDashboard(provider).Page, "/app").Body.String()

	for _, want := range []string{"0596111111", "0696222222", "Rappeler pour le devis"} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	// The number titles the row, so the detail line must not repeat it.
	if strings.Count(body, "0696222222") != 1 {
		t.Error("the task phone number is rendered twice")
	}
}

type pendingFollowUpsStub struct {
	pending []followup.Pending
	err     error
}

func (s *pendingFollowUpsStub) Pending(context.Context) ([]followup.Pending, error) {
	return s.pending, s.err
}

// The "à traiter" panel was empty from the first day: the DTO existed, the data
// did not. This is the mapping that fills it.
func TestDashboardShowsPendingFollowUps(t *testing.T) {
	base := &stubProvider{data: views.Today{Day: testDay}}
	pending := &pendingFollowUpsStub{pending: []followup.Pending{
		{
			Request: followup.Request{
				ID: "fu-1", Kind: followup.KindQuote, Phone: "+596696000002",
				Details: "Devis embrayage", CreatedAt: testDay,
			},
			CustomerName: "Marie Lubin",
		},
		{
			Request: followup.Request{
				ID: "fu-2", Kind: followup.KindCallback, Phone: "+596696000003",
				Details: "Rappeler après 17h", CreatedAt: testDay,
			},
		},
	}}

	provider := NewTodayWithFollowUpsProvider(base, pending)
	body := get(t, newTestDashboard(provider).Page, "/app").Body.String()

	for _, want := range []string{"Marie Lubin", "Devis embrayage", "Devis", "Rappeler après 17h", "+596696000003", "Rappel"} {
		if !strings.Contains(body, want) {
			t.Errorf("the tasks panel is missing %q", want)
		}
	}
	// An unknown caller is titled by its number, never by an invented name.
	if strings.Contains(body, "Numéro inconnu") {
		t.Error("a row with a phone number was titled as unknown")
	}
}

// A failing follow-up read must not silently show an empty queue: the dashboard
// degrades as a whole, which is the behaviour F04 already guarantees.
func TestDashboardFollowUpFailureDegradesThePage(t *testing.T) {
	base := &stubProvider{data: views.Today{Day: testDay}}
	pending := &pendingFollowUpsStub{err: errors.New("database is down")}

	body := get(t, newTestDashboard(NewTodayWithFollowUpsProvider(base, pending)).Page, "/app").Body.String()
	if !strings.Contains(body, "momentanément indisponibles") {
		t.Error("the page does not explain the failure")
	}
	if strings.Contains(body, "database is down") {
		t.Error("the page leaks the backend error")
	}
}
