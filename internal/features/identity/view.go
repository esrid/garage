package identity

import "strings"

// Login is what GET /login renders. The page holds no session state of its own:
// F09 owns cookies, hashing and lockouts, and this view only ever shows a form
// and, at most, one message.
type Page struct {
	// Next is the app path to return to after a successful login, as encoded by
	// the F09 middleware. Empty when the visitor came to /login directly.
	Next string
	// Error is the closed code carried by ?error=. Anything unknown renders the
	// generic message: an unexpected code must not leak as raw text on the page.
	Error string
}

// loginMessages are the only messages this page shows.
//
// "invalid" stays deliberately vague. The F09 contract requires that a failure
// never reveal whether an email exists, is disabled or is locked, and a friendlier
// "unknown account" here would undo that server-side care.
var loginMessages = map[string]string{
	"invalid":      "Email ou mot de passe incorrect.",
	"rate_limited": "Trop de tentatives. Patientez une minute avant de réessayer.",
	"unavailable":  "Service momentanément indisponible. Réessayez dans un instant.",
	"forbidden":    "Session expirée pendant l'envoi du formulaire. Recommencez.",
	"expired":      "Votre session a expiré. Reconnectez-vous.",
}

// LoginMessage is the sentence to show, or "" when there is nothing to say.
func (l Page) LoginMessage() string {
	code := strings.TrimSpace(l.Error)
	if code == "" {
		return ""
	}
	if message, ok := loginMessages[code]; ok {
		return message
	}
	return loginMessages["invalid"]
}

// Expired reports whether the visitor was sent here by an expired session rather
// than by a failed attempt. The page then explains the redirect instead of
// looking like a rejection.
func (l Page) Expired() bool {
	return strings.TrimSpace(l.Error) == "expired"
}
