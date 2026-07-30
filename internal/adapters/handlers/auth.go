package handlers

import (
	"context"
	"errors"
	"net/http"
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
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid login request", http.StatusUnprocessableEntity)
		return
	}
	email := strings.TrimSpace(r.PostForm.Get("email"))
	password := r.PostForm.Get("password")
	if email == "" || password == "" || len([]byte(password)) > coreauth.MaxPasswordBytes {
		http.Error(w, "invalid login request", http.StatusUnprocessableEntity)
		return
	}
	release, retryAfter, allowed := h.limiter.acquire()
	if !allowed {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}
	defer release()

	session, err := h.service.Login(r.Context(), email, password)
	if err != nil {
		writeAuthenticationError(w, err)
		return
	}
	http.SetCookie(w, sessionCookie(session.Token, session.ExpiresAt))
	http.Redirect(w, r, "/app", http.StatusSeeOther)
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

func isFormContentType(value string) bool {
	return strings.EqualFold(strings.TrimSpace(strings.Split(value, ";")[0]), "application/x-www-form-urlencoded")
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
