package usecase

import (
	"coffee-reel/entity"
	"coffee-reel/repository"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

type RateLimitDecision struct {
	Allowed    bool
	Remaining  float64
	RetryAfter time.Duration
}

type IRateLimitUsecase interface {
	AllowSignup(ctx context.Context, clientIP string) (RateLimitDecision, error)
	AllowLoginIP(ctx context.Context, clientIP string) (RateLimitDecision, error)
	AllowLoginEmail(ctx context.Context, normalizedEmail string) (RateLimitDecision, error)
	AllowRefresh(ctx context.Context, plainRefreshToken string) (RateLimitDecision, error)
}

type rateLimitUsecase struct {
	rateLimits       repository.IRateLimitRepository
	tokens           ITokenService
	rateLimitHMACKey []byte
}

const (
	rateLimitCost = 1.0

	signupRate     = 1.0 / 10.0
	signupCapacity = 3.0
	signupTTL      = 60 * time.Second

	loginIPRate     = 1.0 / 5.0
	loginIPCapacity = 10.0
	loginIPTTL      = 100 * time.Second

	loginEmailRate     = 1.0 / 10.0
	loginEmailCapacity = 5.0
	loginEmailTTL      = 100 * time.Second

	refreshRate     = 1.0 / 5.0
	refreshCapacity = 5.0
	refreshTTL      = 60 * time.Second
)

func NewRateLimitUsecase(rateLimits repository.IRateLimitRepository, tokens ITokenService, rateLimitHMACKey string) (IRateLimitUsecase, error) {
	if len([]byte(rateLimitHMACKey)) < minimumSecretSize {
		return nil, fmt.Errorf("rate limit HMAC key must be at least 32 bytes")
	}
	return &rateLimitUsecase{rateLimits: rateLimits, tokens: tokens, rateLimitHMACKey: []byte(rateLimitHMACKey)}, nil
}

// Client IPからSignup用Redis Keyを作成する
func (u *rateLimitUsecase) AllowSignup(ctx context.Context, clientIP string) (RateLimitDecision, error) {
	ip, err := normalizeClientIP(clientIP)
	if err != nil {
		return RateLimitDecision{}, err
	}
	return u.allow(ctx, "rl:signup:ip:"+ip, signupRate, signupCapacity, signupTTL)

}

// Client IPからLogin IP用Keyを作成する
func (u *rateLimitUsecase) AllowLoginIP(ctx context.Context, clientIP string) (RateLimitDecision, error) {
	ip, err := normalizeClientIP(clientIP)

	if err != nil {
		return RateLimitDecision{}, err
	}

	return u.allow(ctx, "rl:login:ip:"+ip, loginIPRate, loginIPCapacity, loginIPTTL)
}

// 正規化済みEmailを専用鍵でHMAC-SHA-256化する
func (u *rateLimitUsecase) AllowLoginEmail(ctx context.Context, normalizedEmail string) (RateLimitDecision, error) {
	if normalizedEmail == "" {
		return RateLimitDecision{}, fmt.Errorf("normalized email is required")
	}
	mac := hmac.New(sha256.New, u.rateLimitHMACKey)
	mac.Write([]byte(normalizedEmail))

	emailHash := hex.EncodeToString(mac.Sum(nil))
	return u.allow(ctx, "rl:login:email:"+emailHash, loginEmailRate, loginEmailCapacity, loginEmailTTL)
}

// token.goのRefresh Token HashをKeyに使用する
func (u *rateLimitUsecase) AllowRefresh(ctx context.Context, plainRefreshToken string) (RateLimitDecision, error) {
	if plainRefreshToken == "" {
		return RateLimitDecision{}, entity.ErrRefreshTokenMissing
	}

	tokenHash := u.tokens.HashRefreshToken(plainRefreshToken)

	return u.allow(ctx, "rl:refresh:token:"+tokenHash, refreshRate, refreshCapacity, refreshTTL)
}
func (u *rateLimitUsecase) allow(ctx context.Context, key string, rate, capacity float64, ttl time.Duration) (RateLimitDecision, error) {
	allowed, remaining, retryAfterMS, err := u.rateLimits.Allow(ctx, key, rate, capacity, rateLimitCost, time.Now().UTC().UnixMilli(), ttl.Milliseconds())
	if err != nil {
		return RateLimitDecision{}, err
	}

	return RateLimitDecision{Allowed: allowed, Remaining: remaining, RetryAfter: time.Duration(retryAfterMS) * time.Millisecond}, nil
}

// IPを検証・正規化し、空文字の共通Key生成を防ぐ
func normalizeClientIP(clientIP string) (string, error) {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return "", fmt.Errorf("client IP is required")
	}

	ip, err := netip.ParseAddr(clientIP)
	if err != nil {
		return "", fmt.Errorf("invalid client IP")
	}

	return ip.Unmap().String(), nil
}
