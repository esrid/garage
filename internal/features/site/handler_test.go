package site

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/esrid/garage/internal/web/views"
)

// siteMux is the public site as the router mounts it, so these tests exercise
// the real patterns instead of calling the handlers directly.
func siteMux() *http.ServeMux {
	mux := http.NewServeMux()
	NewSite().Register(mux)
	// The shell links to /login, which the identity feature serves. The harness
	// answers it so the "no internal link 404s" check measures this feature, not
	// the absence of its neighbour.
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func fetch(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Host = "atelier.example"
	response := httptest.NewRecorder()
	siteMux().ServeHTTP(response, request)
	return response
}

func TestEverySitePageRendersItsSEOHead(t *testing.T) {
	for path, page := range sitePages {
		t.Run(path, func(t *testing.T) {
			response := fetch(t, path)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			body := response.Body.String()

			canonical := "http://atelier.example" + path
			// templ escapes the copy, apostrophes included. Escaping the expected
			// value with templ's own function keeps the two from drifting apart.
			for _, want := range []string{
				"<title>" + templ.EscapeString(page.title) + "</title>",
				`name="description" content="` + templ.EscapeString(page.description) + `"`,
				`rel="canonical" href="` + canonical + `"`,
				`property="og:url" content="` + canonical + `"`,
				`property="og:title"`,
				`"@type":"Organization"`,
				`<html lang="fr">`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("page is missing %s", want)
				}
			}
			// A page whose body failed to render would still have a head.
			if !strings.Contains(body, "<h1") {
				t.Error("page has no h1")
			}
		})
	}
}

// The home page must be an exact match: "GET /" would otherwise answer 200 for
// every unknown path and hand crawlers infinite duplicate pages.
func TestUnknownPathIsNotFound(t *testing.T) {
	if code := fetch(t, "/tarifs-2024").Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

// Nothing in the shell may point at a route that does not exist.
func TestNoInternalLinkIs404(t *testing.T) {
	links := regexp.MustCompile(`href="(/[^"#]*)"`)
	mux := siteMux()

	for path := range sitePages {
		body := fetch(t, path).Body.String()
		for _, match := range links.FindAllStringSubmatch(body, -1) {
			target := match[1]
			if strings.HasPrefix(target, "/static/") {
				continue // served by the router, not by the site table
			}
			request := httptest.NewRequest(http.MethodGet, target, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Errorf("%s links to %s which answers %d", path, target, response.Code)
			}
		}
	}
}

// A call to action that points at the page you are already on wastes the only
// click a prospect gives you. The nav and the brand may self-link (that is what
// aria-current is for); buttons may not.
func TestNoCallToActionPointsAtItsOwnPage(t *testing.T) {
	buttons := regexp.MustCompile(`<a class="btn[^"]*" href="([^"]+)"`)

	for path := range sitePages {
		body := fetch(t, path).Body.String()
		for _, match := range buttons.FindAllStringSubmatch(body, -1) {
			if match[1] == path {
				t.Errorf("%s has a button linking to itself", path)
			}
		}
	}
}

// The nav and the footer are declared in views; every entry must be a real page.
func TestNavAndFooterMatchTheRouteTable(t *testing.T) {
	for _, link := range slices.Concat(views.SiteNav, views.SiteLegal) {
		if _, ok := sitePages[link.Path]; !ok {
			t.Errorf("%q (%s) has no page in the route table", link.Path, link.Label)
		}
	}
}

func TestRobotsKeepsCrawlersOutOfTheApp(t *testing.T) {
	response := fetch(t, "/robots.txt")
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("content type = %q", got)
	}
	body := response.Body.String()
	for _, want := range []string{
		"User-agent: *",
		// Without the trailing slash: "/app/" leaves the dashboard at /app itself
		// open to crawlers, since Disallow matches a path prefix.
		"Disallow: /app\n",
		"Disallow: /voice\n",
		"Sitemap: http://atelier.example/sitemap.xml",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("robots.txt is missing %q", want)
		}
	}
}

func TestSitemapListsEveryPublicPageAbsolutely(t *testing.T) {
	response := fetch(t, "/sitemap.xml")
	var set sitemapURLSet
	if err := xml.Unmarshal(response.Body.Bytes(), &set); err != nil {
		t.Fatalf("sitemap is not valid XML: %v", err)
	}
	if len(set.URLs) != len(sitePages) {
		t.Fatalf("sitemap has %d urls, want %d", len(set.URLs), len(sitePages))
	}

	locations := make([]string, 0, len(set.URLs))
	for _, entry := range set.URLs {
		if !strings.HasPrefix(entry.Loc, "http://atelier.example/") {
			t.Errorf("loc %q is not absolute", entry.Loc)
		}
		if strings.Contains(entry.Loc, "/app") || strings.Contains(entry.Loc, "/voice") {
			t.Errorf("loc %q exposes a private route", entry.Loc)
		}
		locations = append(locations, entry.Loc)
	}
	// Sorted output keeps the document byte-identical between two requests.
	if !slices.IsSorted(locations) {
		t.Errorf("sitemap is not sorted: %v", locations)
	}
}

// Behind the reverse proxy the canonical URL must be the https one the visitor
// used, not the http hop between proxy and app.
func TestCanonicalFollowsTheForwardedScheme(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/tarifs", nil)
	request.Host = "atelier.example"
	request.Header.Set("X-Forwarded-Proto", "https, http")
	response := httptest.NewRecorder()
	siteMux().ServeHTTP(response, request)

	if want := `rel="canonical" href="https://atelier.example/tarifs"`; !strings.Contains(response.Body.String(), want) {
		t.Errorf("canonical is not %s", want)
	}
}

// Prices and quotas are the founder's numbers (PRD §1). A page that lost them,
// or grew a number nobody validated, is a commercial bug.
func TestPricingShowsTheAgreedNumbers(t *testing.T) {
	body := fetch(t, "/tarifs").Body.String()
	for _, want := range []string{"349 €", "599 €", "750 minutes", "1 750 minutes", "[À VALIDER]"} {
		if !strings.Contains(body, want) {
			t.Errorf("pricing page is missing %q", want)
		}
	}
}

// The login page is the identity feature's, but robots.txt is served here.
func TestRobotsKeepsCrawlersOffTheLoginForm(t *testing.T) {
	if body := fetch(t, "/robots.txt").Body.String(); !strings.Contains(body, "Disallow: /login") {
		t.Error("robots.txt does not disallow /login")
	}
}
