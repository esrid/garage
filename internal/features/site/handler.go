package site

import (
	"encoding/xml"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/a-h/templ"

	"github.com/esrid/garage/internal/web/page"
	"github.com/esrid/garage/internal/web/views"
)

// Site serves the public site: marketing pages, legal pages, robots.txt and
// sitemap.xml (PRD §11).
//
// It has no dependencies. Every page is static HTML built from the table below,
// so nothing here can fail at runtime, and nothing here touches a tenant.
type Site struct{}

func NewSite() *Site { return &Site{} }

type sitePage struct {
	// title is the exact <title>, brand suffix included: no assembly at render
	// time, so what you read here is what a search result shows.
	title       string
	description string
	body        func() templ.Component
}

// sitePages is the whole public site. Adding a page means adding one entry:
// routing, the sitemap and the "no footer link 404s" test all read this table.
var sitePages = map[string]sitePage{
	"/": {
		title:       "Atelier IA — l'assistant qui répond au téléphone du garage",
		description: "L'assistant décroche, reconnaît le client et pose le rendez-vous dans votre planning. Pour les garages indépendants en Martinique.",
		body:        SiteHome,
	},
	"/fonctionnalites": {
		title:       "Fonctionnalités — Atelier IA",
		description: "Appels répondus, plaque confirmée, rendez-vous posés au planning, transfert vers un humain, résumé de chaque appel.",
		body:        SiteFeatures,
	},
	"/tarifs": {
		title:       "Tarifs — Atelier IA",
		description: "Deux offres à 349 € et 599 € HT par mois, minutes incluses. Pas de forfait illimité, et une alerte avant la fin du quota.",
		body:        SitePricing,
	},
	"/garages": {
		title:       "Pour les garages — Atelier IA",
		description: "Pensé pour les garages indépendants et les petits ateliers. Le vocabulaire de l'atelier, pas celui d'un logiciel généraliste.",
		body:        SiteWorkshops,
	},
	"/demo": {
		title:       "Démo — Atelier IA",
		description: "Une démo, c'est un appel : vous appelez un numéro de test, vous jouez le client, vous voyez le rendez-vous arriver.",
		body:        SiteDemo,
	},
	"/contact": {
		title:       "Contact — Atelier IA",
		description: "Parler à quelqu'un qui connaît le produit et l'atelier. Un interlocuteur unique, sans formulaire inutile.",
		body:        SiteContact,
	},
	"/mentions-legales": {
		title:       "Mentions légales — Atelier IA",
		description: "Éditeur du site, hébergeur et propriété intellectuelle.",
		body:        SiteLegalNotice,
	},
	"/confidentialite": {
		title:       "Politique de confidentialité — Atelier IA",
		description: "Quelles données l'assistant traite, pourquoi, combien de temps, et comment exercer vos droits.",
		body:        SitePrivacy,
	},
	"/cgv": {
		title:       "Conditions générales de vente — Atelier IA",
		description: "Prestations couvertes par l'abonnement, prix, quota de minutes et fin de contrat.",
		body:        SiteTermsOfSale,
	},
	"/cgu": {
		title:       "Conditions générales d'utilisation — Atelier IA",
		description: "Les règles d'usage du service et les responsabilités de chacun.",
		body:        SiteTermsOfUse,
	},
}

// Register mounts the public site on mux.
//
// The routes live here rather than in the router so the table above stays the
// single place a page is declared.
func (s *Site) Register(mux *http.ServeMux) {
	for path, entry := range sitePages {
		pattern := "GET " + path
		if path == "/" {
			// {$} is an exact match: without it "GET /" would swallow every
			// unmatched path and answer the home page with a 200 instead of a 404.
			pattern = "GET /{$}"
		}
		mux.HandleFunc(pattern, s.render(path, entry))
	}
	mux.HandleFunc("GET /robots.txt", s.robots)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
}

func (s *Site) render(path string, entry sitePage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		meta := views.SiteMeta{
			Title:       entry.title,
			Description: entry.description,
			Origin:      page.Origin(r),
			Path:        path,
		}
		// Identical for every visitor, so a shared cache is safe. Five minutes is
		// short enough that a corrected sentence goes live quickly.
		w.Header().Set("Cache-Control", "public, max-age=300")
		// templ.Handler buffers, so a mid-render error cannot emit half a page
		// under a 200.
		templ.Handler(views.SiteLayout(meta, entry.body())).ServeHTTP(w, r)
	}
}

func (s *Site) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	// Keeping crawlers out of the application and the voice tools is hygiene, not
	// access control: those routes are also behind authentication.
	//
	// No trailing slash: Disallow matches on prefix, so "/app" covers /app itself
	// and everything under it. "/app/" would have left the dashboard crawlable.
	// /login is public but worthless in an index, and a crawler hitting a
	// credentials form is noise in the logs.
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /app\nDisallow: /voice\nDisallow: /login\nSitemap: %s/sitemap.xml\n", page.Origin(r))
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

func (s *Site) sitemap(w http.ResponseWriter, r *http.Request) {
	root := page.Origin(r)
	// Sorted paths: two requests produce byte-identical XML, so a crawler sees a
	// diff only when a page really changed. No lastmod and no priority — we have
	// no publication date to state and would only be inventing one.
	set := sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, path := range slices.Sorted(maps.Keys(sitePages)) {
		set.URLs = append(set.URLs, sitemapURL{
			Loc: views.SiteMeta{Origin: root, Path: path}.Canonical(),
		})
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(xml.Header))
	// The encoder escapes the URLs; hand-written XML here would be a quoting bug
	// waiting for the first ampersand.
	_ = xml.NewEncoder(w).Encode(set)
}
