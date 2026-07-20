package usecase

import (
	"coffee-reel/entity"
	"coffee-reel/repository"
	"context"
	"time"
)

type IUserUsecase interface {
	SignUp(ctx context.Context, name, email, password string) (*entity.User, error)
	Login(ctx context.Context, email, password string) (user *entity.User, accessToken string, refreshToken string, csrfToken string, refreshTokenExpiresAt time.Time, err error)
	Refresh(ctx context.Context, plainRefreshToken string) (accessToken string, refreshToken string, csrfToken string, refreshTokenExpiresAt time.Time, err error)
	Logout(ctx context.Context, refreshToken string) error
	GetMe(ctx context.Context, userID uint64) (*entity.User, error)
	ValidateTokenVersion(user *entity.User, tokenVersion uint64) error
}

type userUsecase struct {
	users         repository.IUserRepository
	refreshTokens entity.RefreshToken
	tokens        ITokenService
}

func NewUserUsecase(users repository.IUserRepository, refreshTokens repository.IRefreshTokenRepository, tokens ITokenService) IUserUsecase {
	return &userUsecase{users: users, refreshTokens: refreshTokens, tokens: tokens}
}
