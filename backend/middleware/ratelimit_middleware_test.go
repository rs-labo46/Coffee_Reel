package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"coffee-reel/usecase"

	"github.com/labstack/echo/v4"
)

func newRateLimitContext(method, path, remoteAddr string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set(echo.HeaderXRequestID, "request-rate")
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestRateLimitMiddlewareAllowsRequestAndUsesDirectClientIP(t *testing.T) {
	calledIP := ""
	rateLimits := &rateLimitUsecaseMock{allowSignupFunc: func(_ context.Context, clientIP string) (usecase.RateLimitDecision, error) {
		calledIP = clientIP
		return usecase.RateLimitDecision{Allowed: true, Remaining: 2}, nil
	}}
	c, rec := newRateLimitContext(http.MethodPost, "/signup", "192.0.2.10:4321")
	c.Request().Header.Set(echo.HeaderXForwardedFor, "203.0.113.100")
	nextCalled := false

	err := NewRateLimitMiddleware(rateLimits).Signup(func(c echo.Context) error {
		nextCalled = true
		return c.NoContent(http.StatusNoContent)
	})(c)
	if err != nil {
		t.Fatalf("Signup() returned Echo error = %v", err)
	}
	if !nextCalled || rec.Code != http.StatusNoContent {
		t.Fatalf("nextCalled=%v status=%d", nextCalled, rec.Code)
	}
	if calledIP != "192.0.2.10" {
		t.Fatalf("AllowSignup IP = %q, want direct peer IP and not untrusted X-Forwarded-For", calledIP)
	}
}

func TestRateLimitMiddlewareBlocksBeforeControllerAndRoundsRetryAfterUp(t *testing.T) {
	rateLimits := &rateLimitUsecaseMock{allowLoginIPFunc: func(context.Context, string) (usecase.RateLimitDecision, error) {
		return usecase.RateLimitDecision{Allowed: false, RetryAfter: 1501 * time.Millisecond}, nil
	}}
	c, rec := newRateLimitContext(http.MethodPost, "/login", "192.0.2.20:1234")
	nextCalled := false

	err := NewRateLimitMiddleware(rateLimits).LoginIP(func(echo.Context) error {
		nextCalled = true
		return nil
	})(c)
	if err != nil {
		t.Fatalf("LoginIP() returned Echo error = %v", err)
	}
	if nextCalled {
		t.Fatal("controller was called after rate limit denial")
	}
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("status=%d Retry-After=%q body=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"rate_limit_exceeded"`) || !strings.Contains(rec.Body.String(), `"request_id":"request-rate"`) {
		t.Fatalf("error response = %s", rec.Body.String())
	}
}

func TestRateLimitMiddlewareFailsClosedWhenRedisOrClientIdentityIsUnavailable(t *testing.T) {
	redisErr := errors.New("redis unavailable")

	t.Run("redis failure", func(t *testing.T) {
		rateLimits := &rateLimitUsecaseMock{allowSignupFunc: func(context.Context, string) (usecase.RateLimitDecision, error) {
			return usecase.RateLimitDecision{}, redisErr
		}}
		c, rec := newRateLimitContext(http.MethodPost, "/signup", "192.0.2.1:1234")
		nextCalled := false
		err := NewRateLimitMiddleware(rateLimits).Signup(func(echo.Context) error { nextCalled = true; return nil })(c)
		if err != nil {
			t.Fatalf("Signup() error = %v", err)
		}
		if nextCalled || rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("nextCalled=%v status=%d", nextCalled, rec.Code)
		}
		if strings.Contains(rec.Body.String(), redisErr.Error()) {
			t.Fatalf("Redis error leaked: %s", rec.Body.String())
		}
	})

	t.Run("invalid client IP", func(t *testing.T) {
		c, rec := newRateLimitContext(http.MethodPost, "/signup", "not-an-ip")
		nextCalled := false
		err := NewRateLimitMiddleware(&rateLimitUsecaseMock{}).Signup(func(echo.Context) error { nextCalled = true; return nil })(c)
		if err != nil {
			t.Fatalf("Signup() error = %v", err)
		}
		if nextCalled || rec.Code != http.StatusInternalServerError {
			t.Fatalf("nextCalled=%v status=%d", nextCalled, rec.Code)
		}
	})
}

func TestRefreshRateLimitRequiresCookieAndDoesNotExposeItsValue(t *testing.T) {
	t.Run("missing cookie", func(t *testing.T) {
		c, rec := newRateLimitContext(http.MethodPost, "/refresh", "192.0.2.1:1234")
		nextCalled := false
		err := NewRateLimitMiddleware(&rateLimitUsecaseMock{}).Refresh(func(echo.Context) error { nextCalled = true; return nil })(c)
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if nextCalled || rec.Code != http.StatusUnauthorized {
			t.Fatalf("nextCalled=%v status=%d", nextCalled, rec.Code)
		}
	})

	t.Run("denied", func(t *testing.T) {
		plainToken := "plain-refresh-secret"
		rateLimits := &rateLimitUsecaseMock{allowRefreshFunc: func(_ context.Context, token string) (usecase.RateLimitDecision, error) {
			if token != plainToken {
				t.Fatalf("AllowRefresh token = %q", token)
			}
			return usecase.RateLimitDecision{Allowed: false, RetryAfter: 0}, nil
		}}
		c, rec := newRateLimitContext(http.MethodPost, "/refresh", "192.0.2.1:1234")
		c.Request().AddCookie(&http.Cookie{Name: refreshCookieName, Value: plainToken})
		err := NewRateLimitMiddleware(rateLimits).Refresh(func(echo.Context) error { return nil })(c)
		if err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
			t.Fatalf("status=%d Retry-After=%q", rec.Code, rec.Header().Get("Retry-After"))
		}
		if strings.Contains(rec.Body.String(), plainToken) {
			t.Fatalf("refresh token leaked: %s", rec.Body.String())
		}
	})
}

func TestClientIPAndRetryAfterSecondsBoundaries(t *testing.T) {
	for _, tt := range []struct {
		value time.Duration
		want  int64
	}{
		{value: 0, want: 1},
		{value: time.Nanosecond, want: 1},
		{value: time.Second, want: 1},
		{value: time.Second + time.Nanosecond, want: 2},
	} {
		if got := retryAfterSeconds(tt.value); got != tt.want {
			t.Fatalf("retryAfterSeconds(%s) = %d, want %d", tt.value, got, tt.want)
		}
	}
}
