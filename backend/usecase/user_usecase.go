package usecase

import (
	"coffee-reel/entity"
	"coffee-reel/repository"
	"context"
	"errors"
	"time"
)

type IUserUsecase interface {
	SignUp(ctx context.Context, name, email, password string) (*entity.User, error)
	Login(ctx context.Context, email, password string) (LoginResult, error)
	Refresh(ctx context.Context, plainRefreshToken string) (RefreshResult, error)
	Logout(ctx context.Context, plainRefreshToken string) error
	GetMe(ctx context.Context, userID uint64) (*entity.User, error)
	ValidateTokenVersion(user *entity.User, tokenVersion uint64) error
	IssueCSRFToken() (CSRFTokenResult, error)
}

const refreshTokenLifetime = 7 * 24 * time.Hour

type AuthTokens struct {
	AccessToken           string
	RefreshToken          string
	CSRFToken             string
	RefreshTokenExpiresAt time.Time
}

type CSRFTokenResult struct {
	Token     string
	ExpiresAt time.Time
}

type userUsecase struct {
	users         repository.IUserRepository
	refreshTokens repository.IRefreshTokenRepository
	tokens        ITokenService
}

type LoginResult struct {
	User *entity.User
	AuthTokens
}

type RefreshResult struct {
	User *entity.User
	AuthTokens
}

func NewUserUsecase(users repository.IUserRepository, refreshTokens repository.IRefreshTokenRepository, tokens ITokenService) IUserUsecase {
	return &userUsecase{users: users, refreshTokens: refreshTokens, tokens: tokens}
}

// Email重複を確認し、PasswordをHash化して一般ユーザーを作成する。
func (u *userUsecase) SignUp(ctx context.Context, name, email, password string) (*entity.User, error) {
	_, err := u.users.FindByEmail(ctx, email)

	if err == nil {
		return nil, entity.ErrEmailAlreadyExists
	}

	if !errors.Is(err, entity.ErrUserNotFound) {
		return nil, err
	}

	passwordHash, err := u.tokens.HashPassword(password)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	user := &entity.User{Name: name, Email: email, PasswordHash: passwordHash, Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 0, CreatedAt: now, UpdatedAt: now}

	if err := u.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// 認証情報とUser状態を確認し、新しいログイン系列とToken一式を発行する。
func (u *userUsecase) Login(ctx context.Context, email, password string) (LoginResult, error) {
	user, err := u.users.FindByEmail(ctx, email)

	if err != nil {
		if errors.Is(err, entity.ErrUserNotFound) {
			return LoginResult{}, entity.ErrInvalidCredentials
		}
		return LoginResult{}, err
	}

	if err := u.tokens.ComparePassword(user.PasswordHash, password); err != nil {
		return LoginResult{}, err
	}

	if !user.IsActive() {
		return LoginResult{}, entity.ErrUserSuspended
	}

	familyID, err := u.tokens.GenerateFamilyID()
	if err != nil {
		return LoginResult{}, err
	}
	authTokens, refreshToken, err := u.issueTokens(user, familyID, time.Now())

	if err != nil {
		return LoginResult{}, err
	}

	if err := u.refreshTokens.Create(ctx, refreshToken); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: user, AuthTokens: authTokens}, nil
}

// Refresh Tokenの状態とUser状態を確認し、同じFamilyでTokenをローテーションする。
func (u *userUsecase) Refresh(ctx context.Context, plainRefreshToken string) (RefreshResult, error) {
	if plainRefreshToken == "" {
		return RefreshResult{},
			entity.ErrRefreshTokenMissing
	}

	tokenHash := u.tokens.HashRefreshToken(
		plainRefreshToken,
	)

	currentToken, err := u.refreshTokens.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return RefreshResult{}, err
	}

	now := time.Now()

	switch {
	case currentToken.IsExpired(now):
		return RefreshResult{},
			entity.ErrRefreshTokenExpired

	case currentToken.RevokedAt != nil:
		return RefreshResult{},
			entity.ErrRefreshTokenRevoked

	case currentToken.UsedAt != nil:
		if err := u.refreshTokens.RevokeFamilyAndIncrementTokenVersion(ctx, currentToken.UserID, currentToken.FamilyID, now); err != nil {
			return RefreshResult{}, err
		}

		return RefreshResult{},
			entity.ErrRefreshTokenReused
	}

	user, err := u.users.FindByID(ctx, currentToken.UserID)
	if err != nil {
		return RefreshResult{}, err
	}

	if !user.IsActive() {
		return RefreshResult{}, entity.ErrUserSuspended
	}

	authTokens, nextToken, err := u.issueTokens(user, currentToken.FamilyID, now)
	if err != nil {
		return RefreshResult{}, err
	}

	if err := u.refreshTokens.Rotate(ctx, tokenHash, nextToken, now); err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{User: user, AuthTokens: authTokens}, nil
}

// Refresh TokenからFamily IDを取得し、同じログイン系列を一括失効する。
func (u *userUsecase) Logout(ctx context.Context, plainRefreshToken string) error {
	if plainRefreshToken == "" {
		return nil
	}

	tokenHash := u.tokens.HashRefreshToken(plainRefreshToken)

	storedToken, err := u.refreshTokens.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return err
	}

	return u.refreshTokens.RevokeFamily(ctx, storedToken.FamilyID, time.Now())
}

// 認証済みUserをDBから取得し、現在もactiveであることを確認する。
func (u *userUsecase) GetMe(ctx context.Context, userID uint64) (*entity.User, error) {
	if userID == 0 {
		return nil, entity.ErrUnauthorized
	}
	user, err := u.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, entity.ErrUserNotFound) {
			return nil, entity.ErrUnauthorized
		}
		return nil, err
	}
	if !user.IsActive() {
		return nil, entity.ErrUserSuspended
	}
	return user, nil
}

// JWT発行時のTokenVersionとDBの現在値を比較し、失効済みAccess Tokenを拒否する。
func (u *userUsecase) ValidateTokenVersion(user *entity.User, tokenVersion uint64) error {
	if user == nil || user.TokenVersion != tokenVersion {
		return entity.ErrUnauthorized
	}
	return nil
}

// Browser再読み込み時にCSRF CookieとHeaderを再同期するためのCSRF Tokenを発行する。
func (u *userUsecase) IssueCSRFToken() (CSRFTokenResult, error) {
	token, err := u.tokens.GenerateCSRFToken()
	if err != nil {
		return CSRFTokenResult{}, err
	}

	return CSRFTokenResult{
		Token:     token,
		ExpiresAt: time.Now().Add(refreshTokenLifetime),
	}, nil
}

// Access Token、Refresh Token、CSRF TokenとDB保存用のRefreshToken Entityを生成する。
func (u *userUsecase) issueTokens(user *entity.User, familyID string, now time.Time) (AuthTokens, *entity.RefreshToken, error) {
	accessToken, err := u.tokens.GenerateAccessToken(user.ID, user.Role, user.TokenVersion, now)
	if err != nil {
		return AuthTokens{}, nil, err
	}

	plainRefreshToken, err := u.tokens.GenerateRefreshToken()
	if err != nil {
		return AuthTokens{}, nil, err
	}

	csrfToken, err := u.tokens.GenerateCSRFToken()
	if err != nil {
		return AuthTokens{}, nil, err
	}

	expiresAt := now.Add(refreshTokenLifetime)

	authTokens := AuthTokens{
		AccessToken:           accessToken,
		RefreshToken:          plainRefreshToken,
		CSRFToken:             csrfToken,
		RefreshTokenExpiresAt: expiresAt,
	}

	refreshToken := &entity.RefreshToken{
		UserID:    user.ID,
		TokenHash: u.tokens.HashRefreshToken(plainRefreshToken),
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}

	return authTokens, refreshToken, nil
}
