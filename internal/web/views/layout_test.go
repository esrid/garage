package views

import (
	"context"
	"strings"
	"testing"
)

func renderLayout(t *testing.T, currentPath string) string {
	t.Helper()
	var out strings.Builder
	body := Layout("Aujourd'hui", currentPath)
	if err := body.Render(context.Background(), &out); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return out.String()
}

// The layout carries the accessibility guarantees every page inherits. If one
// of these disappears, every page loses it silently.
func TestLayoutRendersAccessibleShell(t *testing.T) {
	html := renderLayout(t, "/app")

	for _, want := range []string{
		`lang="fr"`,
		`<title>Aujourd&#39;hui</title>`,
		`class="skip-link" href="#main"`,
		`id="main"`,
		`aria-label="Navigation principale"`,
		`href="/static/css/app.css"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("layout is missing %q", want)
		}
	}
}

// Exactly one nav link may claim to be the current page. Now that the nav lists
// several routes, the test names them: a shared prefix must not light up two.
func TestLayoutMarksCurrentNavLink(t *testing.T) {
	const current = ` aria-current="page"`

	today := renderLayout(t, "/app")
	if !strings.Contains(today, `href="/app"`+current) {
		t.Error("on /app the today link is not marked with aria-current")
	}
	if strings.Contains(today, `href="/app/planning"`+current) {
		t.Error("on /app the planning link claims to be the current page")
	}

	planning := renderLayout(t, "/app/planning")
	if !strings.Contains(planning, `href="/app/planning"`+current) {
		t.Error("on /app/planning the planning link is not marked with aria-current")
	}
	if strings.Contains(planning, `href="/app"`+current) {
		t.Error("on /app/planning the today link claims to be the current page")
	}

	// A route the nav does not list marks nothing.
	if other := renderLayout(t, "/app/unlisted"); strings.Contains(other, current) {
		t.Error("a route absent from the nav marked a link as current")
	}
}

// templ escapes interpolated values. Asserting it here keeps the guarantee
// visible: tenant-supplied titles reach this component.
func TestLayoutEscapesTitle(t *testing.T) {
	var out strings.Builder
	if err := Layout("<script>alert(1)</script>", "/app").Render(context.Background(), &out); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(out.String(), "<script>alert(1)</script>") {
		t.Error("the title was not escaped")
	}
}
