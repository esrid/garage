package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/domain"
)

type authenticationStub struct {
	email, password string
	logoutToken     string
	loginCalls      int
	session         coreauth.Session
	err             error
}

func (s *authenticationStub) Login(_ context.Context, email, password string) (coreauth.Session, error) {
	s.email, s.password = email, password
	s.loginCalls++
	return s.session, s.err
}

func (s *authenticationStub) Logout(_ context.Context, token string) error {
	s.logoutToken = token
	return s.err
}

func TestAuthenticationLoginSetsHardenedCookieAndRedirects(t *testing.T) {
	expires := time.Now().Add(coreauth.SessionLifetime).UTC().Truncate(time.Second)
	stub := &authenticationStub{session: coreauth.Session{Token: "opaque-token", ExpiresAt: expires}}
	handler := NewAuthentication(stub)
	form := url.Values{"email": {"garage@example.com"}, "password": {"secret password"}}
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	response := httptest.NewRecorder()

	handler.Login(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/app" {
		t.Fatalf("status=%d location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if stub.email != "garage@example.com" || stub.password != "secret password" {
		t.Fatalf("credentials passed = %q %q", stub.email, stub.password)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != coreauth.SessionCookieName || cookie.Value != "opaque-token" || cookie.Path != "/" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != int(coreauth.SessionLifetime/time.Second) {
		t.Fatalf("session cookie = %#v", cookie)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login response is cacheable")
	}
}

func TestAuthenticationLoginErrorsAreBoundedAndGeneric(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		err         error
		want        int
	}{
		{"content type", "application/json", `{}`, nil, http.StatusUnsupportedMediaType},
		{"missing field", "application/x-www-form-urlencoded", "email=a%40b.example", nil, http.StatusUnprocessableEntity},
		{"bad credentials", "application/x-www-form-urlencoded", "email=a%40b.example&password=wrong", &domain.UnauthorizedError{}, http.StatusUnauthorized},
		{"store unavailable", "application/x-www-form-urlencoded", "email=a%40b.example&password=wrong", errors.New("dsn secret"), http.StatusServiceUnavailable},
		{"oversized", "application/x-www-form-urlencoded", "email=a%40b.example&password=" + strings.Repeat("x", maxAuthFormBytes), nil, http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			NewAuthentication(&authenticationStub{err: test.err}).Login(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%q", response.Code, test.want, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "dsn secret") {
				t.Fatal("internal error leaked")
			}
		})
	}
}

func TestAuthenticationLogoutRevokesAndExpiresCookie(t *testing.T) {
	stub := &authenticationStub{}
	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: "opaque-token"})
	response := httptest.NewRecorder()
	NewAuthentication(stub).Logout(response, request)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" || stub.logoutToken != "opaque-token" {
		t.Fatalf("status=%d location=%q token=%q", response.Code, response.Header().Get("Location"), stub.logoutToken)
	}
	cookie := response.Result().Cookies()[0]
	if cookie.Name != coreauth.SessionCookieName || cookie.MaxAge != -1 || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("expired cookie = %#v", cookie)
	}
}

func TestAuthenticationLoginRateLimitReturnsRetryAfter(t *testing.T) {
	stub := &authenticationStub{session: coreauth.Session{Token: "opaque-token", ExpiresAt: time.Now().Add(time.Hour)}}
	handler := NewAuthentication(stub)
	form := url.Values{"email": {"unknown@example.com"}, "password": {"wrong password"}}.Encode()
	for attempt := 1; attempt <= maxLoginDerivationsPerMinute+1; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handler.Login(response, request)
		want := http.StatusSeeOther
		if attempt > maxLoginDerivationsPerMinute {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, want)
		}
		if want == http.StatusTooManyRequests && response.Header().Get("Retry-After") == "" {
			t.Fatal("429 response is missing Retry-After")
		}
	}
	if stub.loginCalls != maxLoginDerivationsPerMinute {
		t.Fatalf("service login calls = %d, want %d", stub.loginCalls, maxLoginDerivationsPerMinute)
	}
}

func TestLoginLimiterBoundsConcurrencyAndResetsWindow(t *testing.T) {
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter()
	limiter.now = func() time.Time { return now }
	releaseFirst, _, ok := limiter.acquire()
	if !ok {
		t.Fatal("first concurrent login rejected")
	}
	releaseSecond, _, ok := limiter.acquire()
	if !ok {
		t.Fatal("second concurrent login rejected")
	}
	if _, retryAfter, ok := limiter.acquire(); ok || retryAfter <= 0 {
		t.Fatalf("third concurrent login allowed=%v retryAfter=%v", ok, retryAfter)
	}
	releaseFirst()
	releaseSecond()

	for attempt := 2; attempt < maxLoginDerivationsPerMinute; attempt++ {
		release, _, allowed := limiter.acquire()
		if !allowed {
			t.Fatalf("budget attempt %d rejected", attempt+1)
		}
		release()
	}
	if _, retryAfter, ok := limiter.acquire(); ok || retryAfter != loginDerivationWindow {
		t.Fatalf("exhausted budget allowed=%v retryAfter=%v", ok, retryAfter)
	}
	now = now.Add(loginDerivationWindow)
	release, _, ok := limiter.acquire()
	if !ok {
		t.Fatal("new login window did not reset")
	}
	release()
}
