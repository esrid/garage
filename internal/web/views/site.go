package views

// siteName is the product name shown in the tab, the Open Graph tags and the
// schema.org payload. One constant, so the three can never disagree.
const siteName = "Atelier IA"

// SiteMeta is everything the public <head> needs. The handler builds it from the
// page table and the request: a view never guesses its own URL.
type SiteMeta struct {
	Title       string
	Description string
	// Origin is the absolute site root, "https://example.com", no trailing slash.
	Origin string
	// Path is the page path as registered on the mux, "/" for the home page.
	Path string
}

// Canonical is the absolute URL of this page, for <link rel="canonical"> and
// og:url. Both read the same value so they cannot drift apart.
func (m SiteMeta) Canonical() string {
	if m.Path == "/" {
		return m.Origin + "/"
	}
	return m.Origin + m.Path
}

// organizationLD is the schema.org payload. Name and URL are the only facts we
// can state: address, phone and founding date are not validated yet, and an
// invented one would be a lie in a machine-readable format.
//
// A map serialises with sorted keys, so the JSON is byte-identical per request.
func (m SiteMeta) organizationLD() map[string]string {
	return map[string]string{
		"@context": "https://schema.org",
		"@type":    "Organization",
		"name":     siteName,
		"url":      m.Origin + "/",
	}
}

// SiteNav is the public navigation, and the footer's first column. Declared once
// here because the handler also asserts every link resolves to a real route.
var SiteNav = []SiteLink{
	{Path: "/fonctionnalites", Label: "Fonctionnalités"},
	{Path: "/tarifs", Label: "Tarifs"},
	{Path: "/garages", Label: "Garages"},
	{Path: "/demo", Label: "Démo"},
	{Path: "/contact", Label: "Contact"},
}

// SiteLegal is the footer's legal column.
var SiteLegal = []SiteLink{
	{Path: "/mentions-legales", Label: "Mentions légales"},
	{Path: "/confidentialite", Label: "Politique de confidentialité"},
	{Path: "/cgv", Label: "CGV"},
	{Path: "/cgu", Label: "CGU"},
}

type SiteLink struct {
	Path  string
	Label string
}
