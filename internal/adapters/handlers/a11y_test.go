package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/esrid/garage/internal/features/dashboard"
	"github.com/esrid/garage/internal/web/views"
	"regexp"
	"strings"
	"testing"
)

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

// a11yPages is every page under test, rendered.
func a11yPages(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"/app":                         dashboardPage(t),
		"/app/planning":                getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String(),
		"/app/planning?error=conflict": getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning?error=conflict").Body.String(),
		"/":                            fetch(t, "/").Body.String(),
		"/tarifs":                      fetch(t, "/tarifs").Body.String(),
		"/mentions-legales":            fetch(t, "/mentions-legales").Body.String(),
		"/login":                       loginPage(t, "/login?error=invalid&next=/app/planning").Body.String(),
		"/app/calls":                   getCalls(t, fullCallsStub(), "/app/calls").Body.String(),
		"/app/calls/conv-1":            getCalls(t, fullCallsStub(), "/app/calls/conv-1").Body.String(),
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
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String()

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
	body := getPlanning(t, newTestPlanning(fullPlanningStub()).Page, "/app/planning").Body.String()

	fragment := body[strings.Index(body, `id="planning-day"`):]
	for _, want := range []string{`id="planning-duration"`, `id="planning-duration-submit"`} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the swapped fragment has no %s, so focus is lost on swap", want)
		}
	}
}
