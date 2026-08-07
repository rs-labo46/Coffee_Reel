package usecase

import (
	"coffee-reel/entity"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type ITokenService interface {
	HashPassword(password string) (string, error)
	ComparePassword(passwordHash, password string) error
	GenerateAccessToken(userID uint64, role entity.UserRole, tokenVersion uint64, now time.Time) (string, error)
	ParseAccessToken(token string, now time.Time) (AccessTokenClaims, error)
	GenerateRefreshToken() (string, error)
	HashRefreshToken(token string) string
	GenerateFamilyID() (string, error)
	GenerateCSRFToken() (string, error)
	CompareCSRFToken(cookieToken, headerToken string) error
}

// 検証済みAccess Tokenから取得したClaimで、JWTのJSON形式を外部へ漏らさず、認証処理で必要な値だけを保持
type AccessTokenClaims struct {
	UserID       uint64
	Role         entity.UserRole
	TokenVersion uint64
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

// 用途の異なる鍵を分離し、同じ秘密鍵の使い回しを防ぐ。
type tokenService struct {
	jwtSecret           []byte
	refreshTokenHMACKey []byte
}

type accessTokenClaims struct {
	Role         entity.UserRole `json:"role"`
	TokenVersion uint64          `json:"tv"`
	jwt.RegisteredClaims
}

const (
	bcryptCost          = 10
	accessTokenLifetime = 15 * time.Minute
	minimumSecretSize   = 32
	refreshTokenSize    = 32
	familyIDSize        = 16
	csrfTokenSize       = 32
)

// 2つの秘密鍵の長さを確認し、同じ鍵が使い回されていないか確認してtokenServiceを生成する
func NewTokenService(jwtSecret, refreshTokenHMACKey string) (ITokenService, error) {
	if len([]byte(jwtSecret)) < minimumSecretSize {
		return nil, fmt.Errorf("JWT secret must be at least 32 bytes")
	}

	if len([]byte(refreshTokenHMACKey)) < minimumSecretSize {
		return nil, fmt.Errorf("refresh token HMAC key must be at least 32 bytes")
	}

	if hmac.Equal([]byte(jwtSecret), []byte(refreshTokenHMACKey)) {
		return nil, fmt.Errorf("JWT secret and refresh token HMAC key must be different")
	}

	return &tokenService{jwtSecret: []byte(jwtSecret), refreshTokenHMACKey: []byte(refreshTokenHMACKey)}, nil
}

// bcrypt Cost10でPasswordをHash化する
func (s *tokenService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return string(hash), nil
}

// 保存済みPasswordHashと入力Passwordが一致するかbcycriptを使うって確認
func (s *tokenService) ComparePassword(passwordHash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return entity.ErrInvalidCredentials
		}
		return fmt.Errorf("compare password: %w", err)
	}
	return nil
}

// User情報と期限をClaimへ入れ、HS256で署名したAccess Tokenを作る
func (s *tokenService) GenerateAccessToken(userID uint64, role entity.UserRole, tokenVersion uint64, now time.Time) (string, error) {
	if userID == 0 {
		return "", fmt.Errorf("user ID is required")
	}

	if role != entity.RoleUser && role != entity.RoleAdmin {
		return "", fmt.Errorf("invalid user role")
	}

	now = now.UTC()

	claims := accessTokenClaims{
		Role:         role,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenLifetime)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signedToken, nil
}

// JWT形式、署名方式、署名、有効期限、User ID、Roleを検証する
func (s *tokenService) ParseAccessToken(tokenString string, now time.Time) (AccessTokenClaims, error) {
	claims := &accessTokenClaims{}
	parser := jwt.NewParser(jwt.WithStrictDecoding())

	parsedToken, parts, err := parser.ParseUnverified(tokenString, claims)
	if err != nil || parsedToken == nil || parsedToken.Method == nil || len(parts) != 3 || parsedToken.Method.Alg() != jwt.SigningMethodHS256.Alg() {
		return AccessTokenClaims{}, entity.ErrUnauthorized
	}

	signature, err := parser.DecodeSegment(parts[2])
	if err != nil {
		return AccessTokenClaims{}, entity.ErrUnauthorized
	}

	if err := jwt.SigningMethodHS256.Verify(parts[0]+"."+parts[1], signature, s.jwtSecret); err != nil {
		return AccessTokenClaims{}, entity.ErrUnauthorized
	}

	now = now.UTC()

	if claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.IssuedAt.Time.After(now) || now.Compare(claims.ExpiresAt.Time) >= 0 || claims.IssuedAt.Time.Compare(claims.ExpiresAt.Time) >= 0 {
		return AccessTokenClaims{}, entity.ErrUnauthorized
	}

	userID, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || userID == 0 {
		return AccessTokenClaims{}, entity.ErrUnauthorized
	}

	if claims.Role != entity.RoleUser && claims.Role != entity.RoleAdmin {
		return AccessTokenClaims{}, entity.ErrUnauthorized
	}

	return AccessTokenClaims{UserID: userID, Role: claims.Role, TokenVersion: claims.TokenVersion, IssuedAt: claims.IssuedAt.Time.UTC(), ExpiresAt: claims.ExpiresAt.Time.UTC()}, nil
}

// 乱数32bytesからRefresh Tokenを生成する
func (s *tokenService) GenerateRefreshToken() (string, error) {
	return generateRandomToken(refreshTokenSize)
}

// 専用Keyを使用してHMAC-SHA-256を計算し、16進小文字へ変換する
func (s *tokenService) HashRefreshToken(token string) string {
	mac := hmac.New(sha256.New, s.refreshTokenHMACKey)
	mac.Write([]byte(token))

	return hex.EncodeToString(mac.Sum(nil))
}

// 乱数16 bytesからToken系列の識別子を作る
func (s *tokenService) GenerateFamilyID() (string, error) {
	return generateRandomToken(familyIDSize)
}

// 乱数32bytesからCSRF Tokenを作る
func (s *tokenService) GenerateCSRFToken() (string, error) {
	return generateRandomToken(csrfTokenSize)
}

// 空文字を拒否し、一定時間比較で完全一致を確認する
func (s *tokenService) CompareCSRFToken(cookieToken string, headerToken string) error {
	if cookieToken == "" || headerToken == "" || subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
		return entity.ErrCSRFInvalid
	}
	return nil
}

// crypto/randで乱数を生成し、Base64 Raw URL Encodingへ変換する
func generateRandomToken(size int) (string, error) {
	value := make([]byte, size)

	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}
