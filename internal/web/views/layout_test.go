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

func TestLayoutMarksCurrentNavLink(t *testing.T) {
	if current := renderLayout(t, "/app"); !strings.Contains(current, `aria-current="page"`) {
		t.Error("the active route is not marked with aria-current")
	}
	// On another route the same link must not claim to be the current page.
	if other := renderLayout(t, "/app/planning"); strings.Contains(other, `aria-current="page"`) {
		t.Error("a non-active route is marked with aria-current")
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
