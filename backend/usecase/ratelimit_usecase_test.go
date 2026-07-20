package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
)

const testRateLimitHMACKey = "rate-limit-hmac-key-32-bytes-0001"

func TestNewRateLimitUsecaseRejectsShortHMACKey(t *testing.T) {
	usecase, err := NewRateLimitUsecase(&rateLimitRepositoryMock{}, &tokenServiceMock{}, "short")
	if err == nil || usecase != nil {
		t.Fatalf("NewRateLimitUsecase() = (%v, %v), want nil and error", usecase, err)
	}
}

func TestRateLimitUsecaseUsesFixedPoliciesAndSafeKeys(t *testing.T) {
	tests := []struct {
		name         string
		call         func(IRateLimitUsecase) (RateLimitDecision, error)
		wantKey      string
		wantRate     float64
		wantCapacity float64
		wantTTL      int64
	}{
		{
			name: "signup IPv4",
			call: func(u IRateLimitUsecase) (RateLimitDecision, error) {
				return u.AllowSignup(context.Background(), " 192.0.2.10 ")
			},
			wantKey: "rl:signup:ip:192.0.2.10", wantRate: signupRate, wantCapacity: signupCapacity, wantTTL: signupTTL.Milliseconds(),
		},
		{
			name: "login mapped IPv6 is normalized",
			call: func(u IRateLimitUsecase) (RateLimitDecision, error) {
				return u.AllowLoginIP(context.Background(), "::ffff:192.0.2.20")
			},
			wantKey: "rl:login:ip:192.0.2.20", wantRate: loginIPRate, wantCapacity: loginIPCapacity, wantTTL: loginIPTTL.Milliseconds(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			repo := &rateLimitRepositoryMock{allowFunc: func(_ context.Context, key string, rate, capacity, cost float64, nowMS, ttlMS int64) (bool, float64, int64, error) {
				called = true
				if key != tt.wantKey || rate != tt.wantRate || capacity != tt.wantCapacity || cost != 1 || ttlMS != tt.wantTTL {
					t.Fatalf("Allow(%q, %v, %v, %v, %d, %d)", key, rate, capacity, cost, nowMS, ttlMS)
				}
				if nowMS <= 0 {
					t.Fatalf("nowMS = %d", nowMS)
				}
				return true, capacity - 1, 0, nil
			}}
			u, err := NewRateLimitUsecase(repo, &tokenServiceMock{}, testRateLimitHMACKey)
			if err != nil {
				t.Fatalf("NewRateLimitUsecase() error = %v", err)
			}
			decision, err := tt.call(u)
			if err != nil {
				t.Fatalf("rate limit call error = %v", err)
			}
			if !called || !decision.Allowed || decision.RetryAfter != 0 {
				t.Fatalf("decision = %+v, called=%v", decision, called)
			}
		})
	}
}

func TestRateLimitUsecaseHashesEmailAndRefreshTokenIdentifiers(t *testing.T) {
	email := "user@example.com"
	mac := hmac.New(sha256.New, []byte(testRateLimitHMACKey))
	mac.Write([]byte(email))
	wantEmailHash := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name    string
		call    func(IRateLimitUsecase) (RateLimitDecision, error)
		wantKey string
	}{
		{
			name: "email HMAC",
			call: func(u IRateLimitUsecase) (RateLimitDecision, error) {
				return u.AllowLoginEmail(context.Background(), email)
			},
			wantKey: "rl:login:email:" + wantEmailHash,
		},
		{
			name: "refresh HMAC from token service",
			call: func(u IRateLimitUsecase) (RateLimitDecision, error) {
				return u.AllowRefresh(context.Background(), "plain-refresh")
			},
			wantKey: "rl:refresh:token:refresh-hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimitRepositoryMock{allowFunc: func(_ context.Context, key string, _, _, _ float64, _, _ int64) (bool, float64, int64, error) {
				if key != tt.wantKey {
					t.Fatalf("key = %q, want %q", key, tt.wantKey)
				}
				if strings.Contains(key, email) || strings.Contains(key, "plain-refresh") {
					t.Fatalf("rate limit key exposes a plaintext identifier: %q", key)
				}
				return false, 0.25, 1250, nil
			}}
			tokens := &tokenServiceMock{hashRefreshTokenFunc: func(token string) string {
				if token != "plain-refresh" {
					t.Fatalf("HashRefreshToken token = %q", token)
				}
				return "refresh-hash"
			}}
			u, err := NewRateLimitUsecase(repo, tokens, testRateLimitHMACKey)
			if err != nil {
				t.Fatalf("NewRateLimitUsecase() error = %v", err)
			}
			decision, err := tt.call(u)
			if err != nil {
				t.Fatalf("rate limit call error = %v", err)
			}
			if decision.Allowed || decision.Remaining != 0.25 || decision.RetryAfter != 1250*time.Millisecond {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}

func TestRateLimitUsecaseRejectsMissingOrInvalidIdentifiersWithoutRepositoryCall(t *testing.T) {
	repo := &rateLimitRepositoryMock{}
	u, err := NewRateLimitUsecase(repo, &tokenServiceMock{}, testRateLimitHMACKey)
	if err != nil {
		t.Fatalf("NewRateLimitUsecase() error = %v", err)
	}

	if _, err := u.AllowSignup(context.Background(), ""); err == nil {
		t.Fatal("AllowSignup() accepted empty IP")
	}
	if _, err := u.AllowLoginIP(context.Background(), "not-an-ip"); err == nil {
		t.Fatal("AllowLoginIP() accepted invalid IP")
	}
	if _, err := u.AllowLoginEmail(context.Background(), ""); err == nil {
		t.Fatal("AllowLoginEmail() accepted empty email")
	}
	if _, err := u.AllowRefresh(context.Background(), ""); !errors.Is(err, entity.ErrRefreshTokenMissing) {
		t.Fatalf("AllowRefresh() error = %v, want ErrRefreshTokenMissing", err)
	}
}

func TestRateLimitUsecasePropagatesRedisFailure(t *testing.T) {
	redisErr := errors.New("redis unavailable")
	repo := &rateLimitRepositoryMock{allowFunc: func(context.Context, string, float64, float64, float64, int64, int64) (bool, float64, int64, error) {
		return false, 0, 0, redisErr
	}}
	u, err := NewRateLimitUsecase(repo, &tokenServiceMock{}, testRateLimitHMACKey)
	if err != nil {
		t.Fatalf("NewRateLimitUsecase() error = %v", err)
	}
	if _, err := u.AllowSignup(context.Background(), "192.0.2.1"); !errors.Is(err, redisErr) {
		t.Fatalf("AllowSignup() error = %v, want %v", err, redisErr)
	}
}
