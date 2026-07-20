package usecase

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	testJWTSecret       = "jwt-secret-32-bytes-minimum-000001"
	testRefreshHMACKey  = "refresh-hmac-32-bytes-minimum-002"
	otherRefreshHMACKey = "refresh-hmac-32-bytes-minimum-003"
)

func newTestTokenService(t *testing.T) ITokenService {
	t.Helper()
	service, err := NewTokenService(testJWTSecret, testRefreshHMACKey)
	if err != nil {
		t.Fatalf("NewTokenService() error = %v", err)
	}
	return service
}

func TestNewTokenServiceValidatesSeparatedSecrets(t *testing.T) {
	tests := []struct {
		name       string
		jwtSecret  string
		refreshKey string
		wantError  bool
	}{
		{name: "valid distinct secrets", jwtSecret: testJWTSecret, refreshKey: testRefreshHMACKey},
		{name: "short jwt secret", jwtSecret: "short", refreshKey: testRefreshHMACKey, wantError: true},
		{name: "short refresh key", jwtSecret: testJWTSecret, refreshKey: "short", wantError: true},
		{name: "same secret reused", jwtSecret: testJWTSecret, refreshKey: testJWTSecret, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewTokenService(tt.jwtSecret, tt.refreshKey)
			if tt.wantError {
				if err == nil || service != nil {
					t.Fatalf("NewTokenService() = (%v, %v), want error and nil service", service, err)
				}
				return
			}
			if err != nil || service == nil {
				t.Fatalf("NewTokenService() = (%v, %v), want non-nil service", service, err)
			}
		})
	}
}

func TestPasswordHashUsesBcryptCost10AndComparisonDoesNotCollapseInternalErrors(t *testing.T) {
	service := newTestTokenService(t)
	password := "  password  "

	hash, err := service.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == password || strings.Contains(hash, password) {
		t.Fatal("password hash exposes the plaintext password")
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost != 10 {
		t.Fatalf("bcrypt cost = %d, want 10", cost)
	}
	if err := service.ComparePassword(hash, password); err != nil {
		t.Fatalf("ComparePassword(correct) error = %v", err)
	}
	if err := service.ComparePassword(hash, "wrong-password"); !errors.Is(err, entity.ErrInvalidCredentials) {
		t.Fatalf("ComparePassword(wrong) error = %v, want ErrInvalidCredentials", err)
	}
	if err := service.ComparePassword("not-a-bcrypt-hash", password); err == nil || errors.Is(err, entity.ErrInvalidCredentials) {
		t.Fatalf("ComparePassword(malformed hash) error = %v, want internal error distinct from invalid credentials", err)
	}
}

func TestAccessTokenRoundTripAndLifetime(t *testing.T) {
	service := newTestTokenService(t)
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)

	token, err := service.GenerateAccessToken(123, entity.RoleAdmin, 9, now)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	claims, err := service.ParseAccessToken(token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}

	if claims.UserID != 123 || claims.Role != entity.RoleAdmin || claims.TokenVersion != 9 {
		t.Fatalf("claims = %+v, want user=123 role=admin tv=9", claims)
	}
	if !claims.IssuedAt.Equal(now) {
		t.Fatalf("IssuedAt = %s, want %s", claims.IssuedAt, now)
	}
	if !claims.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("ExpiresAt = %s, want %s", claims.ExpiresAt, now.Add(15*time.Minute))
	}
	if claims.IssuedAt.Location() != time.UTC || claims.ExpiresAt.Location() != time.UTC {
		t.Fatal("token timestamps must be returned in UTC")
	}

	if _, err := service.ParseAccessToken(token, now.Add(15*time.Minute)); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("ParseAccessToken(at expiration) error = %v, want ErrUnauthorized", err)
	}
}

func TestAccessTokenRejectsInvalidInputsAndTampering(t *testing.T) {
	service := newTestTokenService(t)
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)

	if _, err := service.GenerateAccessToken(0, entity.RoleUser, 0, now); err == nil {
		t.Fatal("GenerateAccessToken() accepted user ID 0")
	}
	if _, err := service.GenerateAccessToken(1, entity.UserRole("owner"), 0, now); err == nil {
		t.Fatal("GenerateAccessToken() accepted an undefined role")
	}

	valid, err := service.GenerateAccessToken(1, entity.RoleUser, 0, now)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	parts := strings.Split(valid, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d, want 3", len(parts))
	}
	tamperedSignature := []byte(parts[2])
	if tamperedSignature[0] == 'A' {
		tamperedSignature[0] = 'B'
	} else {
		tamperedSignature[0] = 'A'
	}
	parts[2] = string(tamperedSignature)
	if _, err := service.ParseAccessToken(strings.Join(parts, "."), now); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("tampered token error = %v, want ErrUnauthorized", err)
	}

	otherService, err := NewTokenService("different-jwt-secret-32-bytes-00001", otherRefreshHMACKey)
	if err != nil {
		t.Fatalf("NewTokenService(other) error = %v", err)
	}
	if _, err := otherService.ParseAccessToken(valid, now); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("wrong-key token error = %v, want ErrUnauthorized", err)
	}

	hs384 := jwt.NewWithClaims(jwt.SigningMethodHS384, accessTokenClaims{
		Role:         entity.RoleUser,
		TokenVersion: 0,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})
	hs384String, err := hs384.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign HS384 token: %v", err)
	}
	if _, err := service.ParseAccessToken(hs384String, now); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("HS384 token error = %v, want ErrUnauthorized", err)
	}
}

func TestAccessTokenRejectsInvalidClaims(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	service := newTestTokenService(t)

	tests := []struct {
		name   string
		claims accessTokenClaims
	}{
		{
			name: "future issued at",
			claims: accessTokenClaims{Role: entity.RoleUser, RegisteredClaims: jwt.RegisteredClaims{
				Subject: "1", IssuedAt: jwt.NewNumericDate(now.Add(time.Second)), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			}},
		},
		{
			name: "zero subject",
			claims: accessTokenClaims{Role: entity.RoleUser, RegisteredClaims: jwt.RegisteredClaims{
				Subject: "0", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			}},
		},
		{
			name: "non numeric subject",
			claims: accessTokenClaims{Role: entity.RoleUser, RegisteredClaims: jwt.RegisteredClaims{
				Subject: "user", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			}},
		},
		{
			name: "undefined role",
			claims: accessTokenClaims{Role: entity.UserRole("owner"), RegisteredClaims: jwt.RegisteredClaims{
				Subject: "1", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			}},
		},
		{
			name: "missing issued at",
			claims: accessTokenClaims{Role: entity.RoleUser, RegisteredClaims: jwt.RegisteredClaims{
				Subject: "1", ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			}},
		},
		{
			name: "missing expiration",
			claims: accessTokenClaims{Role: entity.RoleUser, RegisteredClaims: jwt.RegisteredClaims{
				Subject: "1", IssuedAt: jwt.NewNumericDate(now),
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, tt.claims)
			tokenString, err := token.SignedString([]byte(testJWTSecret))
			if err != nil {
				t.Fatalf("SignedString() error = %v", err)
			}
			if _, err := service.ParseAccessToken(tokenString, now); !errors.Is(err, entity.ErrUnauthorized) {
				t.Fatalf("ParseAccessToken() error = %v, want ErrUnauthorized", err)
			}
		})
	}
}

func TestRandomTokenSizesEncodingAndUniqueness(t *testing.T) {
	service := newTestTokenService(t)

	tests := []struct {
		name      string
		generate  func() (string, error)
		wantBytes int
	}{
		{name: "refresh token", generate: service.GenerateRefreshToken, wantBytes: 32},
		{name: "family id", generate: service.GenerateFamilyID, wantBytes: 16},
		{name: "csrf token", generate: service.GenerateCSRFToken, wantBytes: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(map[string]struct{}, 64)
			for i := 0; i < 64; i++ {
				value, err := tt.generate()
				if err != nil {
					t.Fatalf("generate token: %v", err)
				}
				decoded, err := base64.RawURLEncoding.DecodeString(value)
				if err != nil {
					t.Fatalf("token is not Raw URL Base64: %v", err)
				}
				if len(decoded) != tt.wantBytes {
					t.Fatalf("decoded length = %d, want %d", len(decoded), tt.wantBytes)
				}
				if _, exists := seen[value]; exists {
					t.Fatalf("duplicate random token generated at iteration %d", i)
				}
				seen[value] = struct{}{}
			}
		})
	}
}

func TestRefreshTokenHashIsDeterministicKeyedAndNotPlainSHA256(t *testing.T) {
	service := newTestTokenService(t)
	other, err := NewTokenService(testJWTSecret, otherRefreshHMACKey)
	if err != nil {
		t.Fatalf("NewTokenService(other) error = %v", err)
	}
	plain := "plain-refresh-token"

	first := service.HashRefreshToken(plain)
	second := service.HashRefreshToken(plain)
	otherHash := other.HashRefreshToken(plain)

	if first != second {
		t.Fatal("same token and key produced different HMAC values")
	}
	if first == otherHash {
		t.Fatal("different HMAC keys produced the same token hash")
	}
	if first == plain || strings.Contains(first, plain) {
		t.Fatal("token hash exposes plaintext")
	}
	if len(first) != 64 {
		t.Fatalf("hash length = %d, want 64 hex characters", len(first))
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Fatalf("hash is not hexadecimal: %v", err)
	}
	if first != strings.ToLower(first) {
		t.Fatalf("hash is not lower-case hexadecimal: %s", first)
	}
}

func TestCompareCSRFTokenRequiresExactNonEmptyMatch(t *testing.T) {
	service := newTestTokenService(t)

	if err := service.CompareCSRFToken("csrf-value", "csrf-value"); err != nil {
		t.Fatalf("matching CSRF tokens error = %v", err)
	}
	for _, tt := range []struct {
		name   string
		cookie string
		header string
	}{
		{name: "missing cookie", cookie: "", header: "csrf-value"},
		{name: "missing header", cookie: "csrf-value", header: ""},
		{name: "different value", cookie: "csrf-value", header: "other"},
		{name: "different case", cookie: "csrf-value", header: "CSRF-VALUE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := service.CompareCSRFToken(tt.cookie, tt.header); !errors.Is(err, entity.ErrCSRFInvalid) {
				t.Fatalf("error = %v, want ErrCSRFInvalid", err)
			}
		})
	}
}
