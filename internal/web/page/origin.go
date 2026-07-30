package page

import (
	"net/http"
	"net/url"
	"strings"
)

// origin is the absolute site root used by canonical, og:url, robots.txt and
// sitemap.xml.
//
// ponytail: derived from the request, not from configuration. Pin an absolute
// site URL in config once the domain is registered — until then a forged Host
// header would show up in the canonical tag of that one response.
func Origin(r *http.Request) string {
	root := url.URL{Scheme: requestScheme(r), Host: r.Host}
	return root.String()
}

// requestScheme trusts the reverse proxy, which is the only component that knows
// how the client connected. A local run without TLS is plain http.
func requestScheme(r *http.Request) string {
	// The header can carry a hop list; the first value is the client-facing one.
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	switch proto = strings.TrimSpace(proto); proto {
	case "http", "https":
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
