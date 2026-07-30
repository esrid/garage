package identity

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

func TestAuthenticationLoginUsesOnlySafeLocalNext(t *testing.T) {
	tests := []struct {
		name string
		next string
		want string
	}{
		{"app", "/app", "/app"},
		{"app child and query", "/app/planning?day=2030-01-02", "/app/planning?day=2030-01-02"},
		{"absolute", "https://attacker.example/app", "/app"},
		{"protocol relative", "//attacker.example/app", "/app"},
		{"lookalike", "/application", "/app"},
		{"surrounding whitespace", " /app/planning ", "/app"},
		{"backslash", `/app\\attacker.example`, "/app"},
		{"encoded backslash", "/app/%5cattacker.example", "/app"},
		{"control", "/app/%0d%0aLocation:%20https://attacker.example", "/app"},
		{"query control", "/app?x=%0d%0aLocation:%20https://attacker.example", "/app"},
		{"query backslash", "/app?x=%5c%5cattacker.example", "/app"},
		{"dot segment", "/app/../", "/app"},
		{"encoded dot segment", "/app/%2e%2e/", "/app"},
		{"fragment", "/app#fragment", "/app"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expires := time.Now().Add(coreauth.SessionLifetime)
			stub := &authenticationStub{session: coreauth.Session{Token: "opaque-token", ExpiresAt: expires}}
			form := url.Values{"email": {"garage@example.com"}, "password": {"secret password"}, "next": {test.next}}
			request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			NewAuthentication(stub).Login(response, request)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != test.want {
				t.Fatalf("status=%d location=%q want=%q", response.Code, response.Header().Get("Location"), test.want)
			}
		})
	}
}

func TestAuthenticationLoginRedirectsBrowserFailuresToClosedCodes(t *testing.T) {
	validBody := url.Values{
		"email": {"garage@example.com"}, "password": {"wrong password"},
		"next": {"/app/planning?day=2030-01-02"},
	}.Encode()
	tests := []struct {
		name        string
		contentType string
		body        string
		err         error
		wantCode    string
	}{
		{"unsupported media", "application/json", `{}`, nil, "invalid"},
		{"invalid form", "application/x-www-form-urlencoded", "email=garage%40example.com", nil, "invalid"},
		{"wrong credentials", "application/x-www-form-urlencoded", validBody, &domain.UnauthorizedError{}, "invalid"},
		{"forbidden", "application/x-www-form-urlencoded", validBody, &domain.ForbiddenError{}, "forbidden"},
		{"unavailable", "application/x-www-form-urlencoded", validBody, errors.New("database secret"), "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			response := httptest.NewRecorder()
			NewAuthentication(&authenticationStub{err: test.err}).Login(response, request)
			if response.Code != http.StatusSeeOther {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			location, err := url.Parse(response.Header().Get("Location"))
			if err != nil || location.Path != "/login" || location.Query().Get("error") != test.wantCode {
				t.Fatalf("location=%q error=%v", response.Header().Get("Location"), err)
			}
			if test.body == validBody && location.Query().Get("next") != "/app/planning?day=2030-01-02" {
				t.Fatalf("next=%q", location.Query().Get("next"))
			}
			if strings.Contains(response.Body.String(), "database") {
				t.Fatal("redirect leaked internal error")
			}
		})
	}
}

func TestAuthenticationLoginHTMXFailureUsesHXRedirect(t *testing.T) {
	form := url.Values{
		"email": {"garage@example.com"}, "password": {"wrong password"},
		"next": {"/app/planning"},
	}.Encode()
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	NewAuthentication(&authenticationStub{err: &domain.UnauthorizedError{}}).Login(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("HX-Redirect"))
	if err != nil || location.Path != "/login" || location.Query().Get("error") != "invalid" || location.Query().Get("next") != "/app/planning" {
		t.Fatalf("HX-Redirect=%q error=%v", response.Header().Get("HX-Redirect"), err)
	}
}

func TestAuthenticationLoginAcceptHTMLIsBrowserNavigation(t *testing.T) {
	form := url.Values{"email": {"garage@example.com"}, "password": {"wrong password"}}.Encode()
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response := httptest.NewRecorder()
	NewAuthentication(&authenticationStub{err: &domain.UnauthorizedError{}}).Login(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?error=invalid" {
		t.Fatalf("status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
}

func TestAuthenticationLoginRateLimitBrowserResponsesKeepRetryAfter(t *testing.T) {
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	handler := NewAuthentication(&authenticationStub{})
	handler.limiter.now = func() time.Time { return now }
	handler.limiter.windowStarted = now
	handler.limiter.attempts = maxLoginDerivationsPerMinute
	form := url.Values{"email": {"garage@example.com"}, "password": {"wrong password"}}.Encode()

	for _, test := range []struct {
		name   string
		header string
		value  string
		want   int
	}{
		{"navigation", "Sec-Fetch-Mode", "navigate", http.StatusSeeOther},
		{"htmx", "HX-Request", "true", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set(test.header, test.value)
			response := httptest.NewRecorder()
			handler.Login(response, request)
			if response.Code != test.want || response.Header().Get("Retry-After") != "60" {
				t.Fatalf("status=%d Retry-After=%q", response.Code, response.Header().Get("Retry-After"))
			}
			redirectHeader := "Location"
			if test.header == "HX-Request" {
				redirectHeader = "HX-Redirect"
			}
			location, err := url.Parse(response.Header().Get(redirectHeader))
			if err != nil || location.Query().Get("error") != "rate_limited" {
				t.Fatalf("%s=%q error=%v", redirectHeader, response.Header().Get(redirectHeader), err)
			}
		})
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
		{"forbidden", "application/x-www-form-urlencoded", "email=a%40b.example&password=wrong", &domain.ForbiddenError{}, http.StatusServiceUnavailable},
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
