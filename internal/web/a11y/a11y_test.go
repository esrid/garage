package a11y_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/features/calls"
	"github.com/esrid/garage/internal/features/dashboard"
	"github.com/esrid/garage/internal/features/identity"
	"github.com/esrid/garage/internal/features/planning"
	"github.com/esrid/garage/internal/features/site"
	"github.com/esrid/garage/internal/web/views"
)

// Package a11y_test asserts the accessibility guarantees that span every page we
// serve. It lives in its own package on purpose: the promise is uniformity - the
// same skip link, the same heading outline, the same "current page" rule on every
// screen - and a suite split per feature could no longer see that.
//
// It reaches each feature the way the router does, through what they export.
//
// The accessibility guarantees of every page we serve, asserted on the rendered
// markup. They were measured once in a browser — focus order, accessible names,
// heading outline, focus ring — and these tests are what keeps them true.
//
// What they cannot replace: a real screen reader and a real Tab key. Those stay
// a manual pass, noted in WORKBOARD.md.

// dashboardPage renders the day view through the dashboard feature's own API.
// This suite asserts a guarantee that spans features, so it reaches them the way
// the router does - through what they export - not through their test doubles.
func dashboardPage(t *testing.T) string {
	t.Helper()
	provider := dashboardStub{data: views.Today{
		Day:          time.Now(),
		Calls:        []views.Call{{ID: "c1", At: time.Now(), Phone: "+596696000001", Outcome: "success"}},
		Appointments: []views.Appointment{{ID: "a1", Start: time.Now(), CustomerName: "Marie Lubin", Status: "confirmed"}},
		Tasks:        []views.Task{{ID: "t1", CreatedAt: time.Now(), Kind: "quote", Phone: "+596696000002", Note: "Devis"}},
	}}
	response := httptest.NewRecorder()
	dashboard.NewDashboard(provider).Page(response, httptest.NewRequest(http.MethodGet, "/app", nil))
	return response.Body.String()
}

type dashboardStub struct {
	data views.Today
}

func (s dashboardStub) Today(context.Context, time.Time) (views.Today, error) {
	return s.data, nil
}

func render(t *testing.T, h http.HandlerFunc, target string) string {
	t.Helper()
	response := httptest.NewRecorder()
	h(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response.Body.String()
}

func renderMux(t *testing.T, mux *http.ServeMux, target string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = "atelier.example"
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response.Body.String()
}

func publicMux() *http.ServeMux {
	mux := http.NewServeMux()
	site.NewSite().Register(mux)
	identity.NewLogin().Register(mux)
	return mux
}

// a11yPages is every page under test, rendered.
func a11yPages(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"/app":                         dashboardPage(t),
		"/app/planning":                render(t, planning.NewHandler(planningStub{}).Page, "/app/planning"),
		"/app/planning?error=conflict": render(t, planning.NewHandler(planningStub{}).Page, "/app/planning?error=conflict"),
		"/":                            renderMux(t, publicMux(), "/"),
		"/tarifs":                      renderMux(t, publicMux(), "/tarifs"),
		"/mentions-legales":            renderMux(t, publicMux(), "/mentions-legales"),
		"/login":                       renderMux(t, publicMux(), "/login?error=invalid&next=/app/planning"),
		"/app/calls":                   render(t, calls.NewCalls(callsStub{}).Day, "/app/calls"),
		"/app/calls/conv-1":            renderMux(t, callsMux(), "/app/calls/conv-1"),
	}
}

// The skip link must be the first focusable element, or a keyboard user has no
// way past the navigation.
func TestEveryPageStartsWithTheSkipLink(t *testing.T) {
	focusable := regexp.MustCompile(`<a\b[^>]*href=|<button\b|<select\b|<summary\b|<input\b`)

	for path, body := range a11yPages(t) {
		first := focusable.FindString(body)
		if first == "" {
			t.Errorf("%s has no focusable element at all", path)
			continue
		}
		// The first thing Tab reaches must be the skip link itself.
		if !strings.Contains(first, "skip-link") {
			t.Errorf("%s reaches %q before the skip link", path, first)
		}
		if !strings.Contains(body, `href="#main"`) || !strings.Contains(body, `id="main"`) {
			t.Errorf("%s has no #main for the skip link to reach", path)
		}
	}
}

// One h1 per page, and no level skipped: the heading outline is how a screen
// reader user navigates a page they cannot scan.
func TestEveryPageHasOneHeadingOutline(t *testing.T) {
	headings := regexp.MustCompile(`<h([1-6])`)

	for path, body := range a11yPages(t) {
		levels := headings.FindAllStringSubmatch(body, -1)
		if len(levels) == 0 {
			t.Errorf("%s has no heading", path)
			continue
		}
		ones, previous := 0, 0
		for _, match := range levels {
			level := int(match[1][0] - '0')
			if level == 1 {
				ones++
			}
			if previous != 0 && level > previous+1 {
				t.Errorf("%s skips from h%d to h%d", path, previous, level)
			}
			previous = level
		}
		if ones != 1 {
			t.Errorf("%s has %d h1, want exactly 1", path, ones)
		}
	}
}

// Two controls acting on different appointments must not share an accessible
// name. Tabbing a day otherwise announces "Annuler le rendez-vous" three times
// with nothing to tell them apart, and someone cancels the wrong vehicle.
func TestPlanningRowControlsAreDistinguishable(t *testing.T) {
	body := render(t, planning.NewHandler(planningStub{}).Page, "/app/planning")

	// Every actionable row carries its own identity, in the summary and on both
	// buttons.
	for _, want := range []string{
		`<span class="visually-hidden"> — Marie Lubin, 09:00</span>`,
		`<span class="visually-hidden"> pour Marie Lubin, 09:00</span>`,
		`<span class="visually-hidden"> le rendez-vous de Marie Lubin, 09:00</span>`,
		`<span class="visually-hidden"> de Marie Lubin, 09:00</span>`,
		`<span class="visually-hidden"> de Jean-Claude Sainte-Rose, 10:30</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a row control is missing its context: %q", want)
		}
	}

	// The generic labels must never appear without a following context span.
	bare := regexp.MustCompile(`>\s*(?:Annuler le rendez-vous|Déplacer)\s*</button>`)
	if match := bare.FindString(body); match != "" {
		t.Errorf("a button carries no row context: %q", match)
	}
}

// Every form control needs a label pointing at its id, or its purpose is unknown
// to anyone who cannot see the text next to it.
func TestEveryFormControlHasALabel(t *testing.T) {
	controls := regexp.MustCompile(`<(?:input|select|textarea)\b[^>]*>`)
	idOf := regexp.MustCompile(`\bid="([^"]+)"`)
	typeOf := regexp.MustCompile(`\btype="([^"]+)"`)

	for path, body := range a11yPages(t) {
		labelled := map[string]bool{}
		for _, match := range regexp.MustCompile(`\bfor="([^"]+)"`).FindAllStringSubmatch(body, -1) {
			labelled[match[1]] = true
		}
		for _, control := range controls.FindAllString(body, -1) {
			if kind := typeOf.FindStringSubmatch(control); kind != nil && kind[1] == "hidden" {
				continue
			}
			if strings.Contains(control, "aria-label=") {
				continue
			}
			id := idOf.FindStringSubmatch(control)
			if id == nil {
				t.Errorf("%s has a control with no id and no aria-label: %s", path, control)
				continue
			}
			if !labelled[id[1]] {
				t.Errorf("%s has no label for %q", path, id[1])
			}
		}
	}
}

// A page reachable from the shell's navigation marks itself once. A page that is
// not in the navigation marks nothing: claiming to be the current nav entry when
// no entry leads here is a lie about where the visitor is.
func TestCurrentPageIsMarkedExactlyOnce(t *testing.T) {
	// The routes the shells put in their navigation. A detail page under one of
	// them marks that section: /app/calls/<id> is still "Appels".
	navigation := []string{"/app", "/app/planning", "/app/calls", "/tarifs", "/login"}

	for path, body := range a11yPages(t) {
		count := strings.Count(body, `aria-current="page"`)
		// The nav marks a route, not a query string: ?error= is the same page.
		route, _, _ := strings.Cut(path, "?")
		want := 0
		if route == "/" {
			want = 1 // the brand is the only link to the home page
		}
		for _, entry := range navigation {
			if strings.HasPrefix(route, entry) {
				want = 1
				break
			}
		}
		if count != want {
			t.Errorf("%s marks %d elements as the current page, want %d", path, count, want)
		}
	}
}

// A positive tabindex reorders the page for keyboard users only, which is how
// focus order stops matching what everyone else reads.
func TestNoPageForcesATabOrder(t *testing.T) {
	positive := regexp.MustCompile(`tabindex="[1-9]`)

	for path, body := range a11yPages(t) {
		if match := positive.FindString(body); match != "" {
			t.Errorf("%s forces a tab order: %s", path, match)
		}
	}
}

// htmx 2.0.10 restores focus after a swap only to an element carrying the same
// id (verified in assets/src/js/htmx-2.0.10.min.js). The control that triggers
// the swap is inside the swapped fragment, so without an id the keyboard user is
// dropped back to the top of the document on every filter change.
func TestSwapTriggersKeepTheirIdentity(t *testing.T) {
	body := render(t, planning.NewHandler(planningStub{}).Page, "/app/planning")

	fragment := body[strings.Index(body, `id="planning-day"`):]
	for _, want := range []string{`id="planning-duration"`, `id="planning-duration-submit"`} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the swapped fragment has no %s, so focus is lost on swap", want)
		}
	}
}

// The doubles below are what the router would inject. They answer with enough
// data for every panel, form and row to render: an empty page proves nothing
// about the accessibility of a page with content.

var martinique = time.FixedZone("AST", -4*60*60)

func at(hour, minute int) time.Time {
	return time.Date(2026, 7, 30, hour, minute, 0, 0, martinique)
}

type planningStub struct{}

func (planningStub) Day(_ context.Context, day time.Time) (appointment.Day, error) {
	local := day.In(martinique)
	return appointment.Day{
		Date:     time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, martinique),
		Timezone: "America/Martinique",
		Openings: []appointment.Opening{{ID: "op-1", Start: at(8, 0), End: at(12, 0), Capacity: 2}},
		Appointments: []appointment.DayEntry{{
			Appointment: appointment.Appointment{
				ID: "rdv-1", Start: at(9, 0), End: at(10, 0),
				ServiceLabel: "Vidange", Status: appointment.StatusConfirmed,
			},
			CustomerName: "Marie Lubin", VehicleLabel: "Clio IV", Plate: "AB-123-CD",
		}, {
			Appointment: appointment.Appointment{
				ID: "rdv-2", Start: at(10, 30), End: at(11, 0),
				ServiceLabel: "Diagnostic", Status: appointment.StatusPending,
			},
			CustomerName: "Jean-Claude Sainte-Rose", VehicleLabel: "Hilux",
		}},
	}, nil
}

func (planningStub) AvailableSlots(_ context.Context, query appointment.AvailabilityQuery) ([]appointment.Slot, error) {
	if int(query.Duration.Minutes()) == 30 {
		return []appointment.Slot{{Start: at(8, 0), End: at(8, 30)}}, nil
	}
	return []appointment.Slot{{Start: at(8, 0), End: at(9, 0)}, {Start: at(11, 0), End: at(12, 0)}}, nil
}

type callsStub struct{}

func (callsStub) Calls(_ context.Context, day time.Time) (views.CallHistory, error) {
	local := day.In(martinique)
	return views.CallHistory{
		Day:      time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, martinique),
		Timezone: "America/Martinique",
		Calls: []views.CallSummary{{
			ID: "conv-1", At: at(8, 12), Duration: 4 * time.Minute,
			CustomerName: "Marie Lubin", Phone: "+596696000001",
			Outcome: "success", Status: "done", Summary: "Vidange prise.",
		}},
	}, nil
}

func (callsStub) Call(context.Context, string) (views.CallDetail, error) {
	return views.CallDetail{
		CallSummary: views.CallSummary{
			ID: "conv-1", At: at(8, 12), Duration: 4 * time.Minute,
			CustomerName: "Marie Lubin", Phone: "+596696000001",
			Outcome: "success", Status: "done", Summary: "Vidange prise.",
		},
		Turns: []views.CallTurn{
			{Role: "agent", Text: "Garage, bonjour.", At: 0},
			{Role: "user", Text: "Une vidange.", At: 4 * time.Second},
		},
	}, nil
}

func callsMux() *http.ServeMux {
	mux := http.NewServeMux()
	calls.NewCalls(callsStub{}).Register(mux)
	return mux
}

// TestWritePreviews dumps the rendered pages so they can be opened in a browser,
// screenshotted, or run through an accessibility reporter. It lives here for the
// same reason the suite does: it spans features. Skipped unless PREVIEW_DIR names
// a directory, because a test suite does not write to disk by default.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR=<directory> to dump the app pages")
	}
	for name, body := range a11yPages(t) {
		file := strings.ReplaceAll(strings.Trim(name, "/"), "/", "-")
		if file == "" {
			file = "home"
		}
		file, _, _ = strings.Cut(file, "?")
		if err := os.WriteFile(filepath.Join(dir, file+".html"), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
}
