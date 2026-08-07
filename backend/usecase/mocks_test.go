package usecase

import (
	"context"
	"testing"
	"time"

	"coffee-reel/entity"
)

type userRepositoryMock struct {
	createFunc      func(context.Context, *entity.User) error
	findByEmailFunc func(context.Context, string) (*entity.User, error)
	findByIDFunc    func(context.Context, uint64) (*entity.User, error)
	updateFunc      func(context.Context, *entity.User) error
}

func (m *userRepositoryMock) Create(ctx context.Context, user *entity.User) error {
	if m.createFunc == nil {
		panic("unexpected UserRepository.Create call")
	}
	return m.createFunc(ctx, user)
}

func (m *userRepositoryMock) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	if m.findByEmailFunc == nil {
		panic("unexpected UserRepository.FindByEmail call")
	}
	return m.findByEmailFunc(ctx, email)
}

func (m *userRepositoryMock) FindByID(ctx context.Context, id uint64) (*entity.User, error) {
	if m.findByIDFunc == nil {
		panic("unexpected UserRepository.FindByID call")
	}
	return m.findByIDFunc(ctx, id)
}

func (m *userRepositoryMock) Update(ctx context.Context, user *entity.User) error {
	if m.updateFunc == nil {
		panic("unexpected UserRepository.Update call")
	}
	return m.updateFunc(ctx, user)
}

type refreshTokenRepositoryMock struct {
	createFunc                               func(context.Context, *entity.RefreshToken) error
	findByTokenHashFunc                      func(context.Context, string) (*entity.RefreshToken, error)
	rotateFunc                               func(context.Context, string, *entity.RefreshToken, time.Time) error
	revokeFamilyFunc                         func(context.Context, string, time.Time) error
	revokeFamilyAndIncrementTokenVersionFunc func(context.Context, uint64, string, time.Time) error
	revokeByUserIDFunc                       func(context.Context, uint64, time.Time) error
	deleteExpiredFunc                        func(context.Context, time.Time) error
}

func (m *refreshTokenRepositoryMock) Create(ctx context.Context, token *entity.RefreshToken) error {
	if m.createFunc == nil {
		panic("unexpected RefreshTokenRepository.Create call")
	}
	return m.createFunc(ctx, token)
}

func (m *refreshTokenRepositoryMock) FindByTokenHash(ctx context.Context, tokenHash string) (*entity.RefreshToken, error) {
	if m.findByTokenHashFunc == nil {
		panic("unexpected RefreshTokenRepository.FindByTokenHash call")
	}
	return m.findByTokenHashFunc(ctx, tokenHash)
}

func (m *refreshTokenRepositoryMock) Rotate(ctx context.Context, tokenHash string, nextToken *entity.RefreshToken, now time.Time) error {
	if m.rotateFunc == nil {
		panic("unexpected RefreshTokenRepository.Rotate call")
	}
	return m.rotateFunc(ctx, tokenHash, nextToken, now)
}

func (m *refreshTokenRepositoryMock) RevokeFamily(ctx context.Context, familyID string, now time.Time) error {
	if m.revokeFamilyFunc == nil {
		panic("unexpected RefreshTokenRepository.RevokeFamily call")
	}
	return m.revokeFamilyFunc(ctx, familyID, now)
}

func (m *refreshTokenRepositoryMock) RevokeFamilyAndIncrementTokenVersion(ctx context.Context, userID uint64, familyID string, now time.Time) error {
	if m.revokeFamilyAndIncrementTokenVersionFunc == nil {
		panic("unexpected RefreshTokenRepository.RevokeFamilyAndIncrementTokenVersion call")
	}
	return m.revokeFamilyAndIncrementTokenVersionFunc(ctx, userID, familyID, now)
}

func (m *refreshTokenRepositoryMock) RevokeByUserID(ctx context.Context, userID uint64, now time.Time) error {
	if m.revokeByUserIDFunc == nil {
		panic("unexpected RefreshTokenRepository.RevokeByUserID call")
	}
	return m.revokeByUserIDFunc(ctx, userID, now)
}

func (m *refreshTokenRepositoryMock) DeleteExpired(ctx context.Context, now time.Time) error {
	if m.deleteExpiredFunc == nil {
		panic("unexpected RefreshTokenRepository.DeleteExpired call")
	}
	return m.deleteExpiredFunc(ctx, now)
}

type tokenServiceMock struct {
	hashPasswordFunc         func(string) (string, error)
	comparePasswordFunc      func(string, string) error
	generateAccessTokenFunc  func(uint64, entity.UserRole, uint64, time.Time) (string, error)
	parseAccessTokenFunc     func(string, time.Time) (AccessTokenClaims, error)
	generateRefreshTokenFunc func() (string, error)
	hashRefreshTokenFunc     func(string) string
	generateFamilyIDFunc     func() (string, error)
	generateCSRFTokenFunc    func() (string, error)
	compareCSRFTokenFunc     func(string, string) error
}

func (m *tokenServiceMock) HashPassword(password string) (string, error) {
	if m.hashPasswordFunc == nil {
		panic("unexpected TokenService.HashPassword call")
	}
	return m.hashPasswordFunc(password)
}

func (m *tokenServiceMock) ComparePassword(passwordHash, password string) error {
	if m.comparePasswordFunc == nil {
		panic("unexpected TokenService.ComparePassword call")
	}
	return m.comparePasswordFunc(passwordHash, password)
}

func (m *tokenServiceMock) GenerateAccessToken(userID uint64, role entity.UserRole, tokenVersion uint64, now time.Time) (string, error) {
	if m.generateAccessTokenFunc == nil {
		panic("unexpected TokenService.GenerateAccessToken call")
	}
	return m.generateAccessTokenFunc(userID, role, tokenVersion, now)
}

func (m *tokenServiceMock) ParseAccessToken(token string, now time.Time) (AccessTokenClaims, error) {
	if m.parseAccessTokenFunc == nil {
		panic("unexpected TokenService.ParseAccessToken call")
	}
	return m.parseAccessTokenFunc(token, now)
}

func (m *tokenServiceMock) GenerateRefreshToken() (string, error) {
	if m.generateRefreshTokenFunc == nil {
		panic("unexpected TokenService.GenerateRefreshToken call")
	}
	return m.generateRefreshTokenFunc()
}

func (m *tokenServiceMock) HashRefreshToken(token string) string {
	if m.hashRefreshTokenFunc == nil {
		panic("unexpected TokenService.HashRefreshToken call")
	}
	return m.hashRefreshTokenFunc(token)
}

func (m *tokenServiceMock) GenerateFamilyID() (string, error) {
	if m.generateFamilyIDFunc == nil {
		panic("unexpected TokenService.GenerateFamilyID call")
	}
	return m.generateFamilyIDFunc()
}

func (m *tokenServiceMock) GenerateCSRFToken() (string, error) {
	if m.generateCSRFTokenFunc == nil {
		panic("unexpected TokenService.GenerateCSRFToken call")
	}
	return m.generateCSRFTokenFunc()
}

func (m *tokenServiceMock) CompareCSRFToken(cookieToken, headerToken string) error {
	if m.compareCSRFTokenFunc == nil {
		panic("unexpected TokenService.CompareCSRFToken call")
	}
	return m.compareCSRFTokenFunc(cookieToken, headerToken)
}

type rateLimitRepositoryMock struct {
	allowFunc func(context.Context, string, float64, float64, float64, int64, int64) (bool, float64, int64, error)
}

func (m *rateLimitRepositoryMock) Allow(ctx context.Context, key string, rate, capacity, cost float64, nowMS, ttlMS int64) (bool, float64, int64, error) {
	if m.allowFunc == nil {
		panic("unexpected RateLimitRepository.Allow call")
	}
	return m.allowFunc(ctx, key, rate, capacity, cost, nowMS, ttlMS)
}

func assertTimeNear(t *testing.T, got, before, after time.Time) {
	t.Helper()
	if got.Before(before) || got.After(after) {
		t.Fatalf("time %s is outside [%s, %s]", got, before, after)
	}
}
