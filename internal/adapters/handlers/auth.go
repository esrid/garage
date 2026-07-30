package handlers

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/domain"
)

const (
	maxAuthFormBytes              = 16 << 10
	maxLoginDerivationsPerMinute  = 30
	maxConcurrentLoginDerivations = 2
	loginDerivationWindow         = time.Minute
)

type Authentication struct {
	service authenticationService
	limiter *loginLimiter
}

type authenticationService interface {
	Login(context.Context, string, string) (coreauth.Session, error)
	Logout(context.Context, string) error
}

func NewAuthentication(service authenticationService) *Authentication {
	return &Authentication{service: service, limiter: newLoginLimiter()}
}

func (h *Authentication) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !isFormContentType(r.Header.Get("Content-Type")) {
		writeLoginFailure(w, r, http.StatusUnsupportedMediaType, "unsupported media type", "invalid", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		writeLoginFailure(w, r, http.StatusUnprocessableEntity, "invalid login request", "invalid", "")
		return
	}
	next := safeLoginNext(r.PostForm.Get("next"))
	email := strings.TrimSpace(r.PostForm.Get("email"))
	password := r.PostForm.Get("password")
	if email == "" || password == "" || len([]byte(password)) > coreauth.MaxPasswordBytes {
		writeLoginFailure(w, r, http.StatusUnprocessableEntity, "invalid login request", "invalid", next)
		return
	}
	release, retryAfter, allowed := h.limiter.acquire()
	if !allowed {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeLoginFailure(w, r, http.StatusTooManyRequests, "too many login attempts", "rate_limited", next)
		return
	}
	defer release()

	session, err := h.service.Login(r.Context(), email, password)
	if err != nil {
		writeLoginServiceError(w, r, err, next)
		return
	}
	http.SetCookie(w, sessionCookie(session.Token, session.ExpiresAt))
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Authentication) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie(coreauth.SessionCookieName)
	if err == nil {
		if err := h.service.Logout(r.Context(), cookie.Value); err != nil {
			writeAuthenticationError(w, err)
			return
		}
	}
	http.SetCookie(w, expiredSessionCookie())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func sessionCookie(token string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     coreauth.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(coreauth.SessionLifetime / time.Second),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

func expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     coreauth.SessionCookieName,
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

// isFormContentType reads the header with mime.ParseMediaType rather than by
// splitting on a semicolon: it is the documented parser, it lowercases the type,
// it understands quoted parameters, and it refuses a malformed value instead of
// silently accepting its prefix.
func isFormContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/x-www-form-urlencoded"
}

func safeLoginNext(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") || containsControl(value) {
		return "/app"
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return "/app"
	}
	if parsed.Path != "/app" && !strings.HasPrefix(parsed.Path, "/app/") {
		return "/app"
	}
	cleanPath := path.Clean(parsed.Path)
	if parsed.Path != cleanPath && parsed.Path != cleanPath+"/" {
		return "/app"
	}
	decodedQuery, err := url.QueryUnescape(parsed.RawQuery)
	if err != nil || strings.Contains(parsed.Path, "\\") || containsControl(parsed.Path) || strings.Contains(decodedQuery, "\\") || containsControl(decodedQuery) {
		return "/app"
	}
	return parsed.RequestURI()
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func writeLoginServiceError(w http.ResponseWriter, r *http.Request, err error, next string) {
	var unauthorized *domain.UnauthorizedError
	var forbidden *domain.ForbiddenError
	switch {
	case errors.As(err, &unauthorized):
		writeLoginFailure(w, r, http.StatusUnauthorized, "invalid email or password", "invalid", next)
	case errors.As(err, &forbidden):
		writeLoginFailure(w, r, http.StatusServiceUnavailable, "authentication service unavailable", "forbidden", next)
	default:
		writeLoginFailure(w, r, http.StatusServiceUnavailable, "authentication service unavailable", "unavailable", next)
	}
}

func writeLoginFailure(w http.ResponseWriter, r *http.Request, status int, message, code, next string) {
	target := loginErrorURL(code, next)
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Request")), "true") {
		w.Header().Set("HX-Redirect", target)
		http.Error(w, message, http.StatusUnauthorized)
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Mode")), "navigate") || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html") {
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	http.Error(w, message, status)
}

func loginErrorURL(code, next string) string {
	query := url.Values{"error": {code}}
	if next != "" && next != "/app" {
		query.Set("next", next)
	}
	return "/login?" + query.Encode()
}

func writeAuthenticationError(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")
	var unauthorized *domain.UnauthorizedError
	if errors.As(err, &unauthorized) {
		http.Error(w, "invalid email or password", http.StatusUnauthorized)
		return
	}
	http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
}

type loginLimiter struct {
	mu            sync.Mutex
	windowStarted time.Time
	attempts      int
	active        chan struct{}
	now           func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		active: make(chan struct{}, maxConcurrentLoginDerivations),
		now:    time.Now,
	}
}

func (l *loginLimiter) acquire() (release func(), retryAfter time.Duration, allowed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if l.windowStarted.IsZero() || !now.Before(l.windowStarted.Add(loginDerivationWindow)) {
		l.windowStarted = now
		l.attempts = 0
	}
	if l.attempts >= maxLoginDerivationsPerMinute {
		return nil, l.windowStarted.Add(loginDerivationWindow).Sub(now), false
	}
	select {
	case l.active <- struct{}{}:
		l.attempts++
		return func() { <-l.active }, 0, true
	default:
		return nil, time.Second, false
	}
}
