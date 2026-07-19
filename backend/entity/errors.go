package entity

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserSuspended       = errors.New("user suspended")
	ErrRefreshTokenMissing = errors.New("refresh token missing")
	ErrRefreshTokenInvalid = errors.New("refresh token invalid")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
	ErrRefreshTokenRevoked = errors.New("refresh token revoked")
	ErrRefreshTokenReused  = errors.New("refresh token reused")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrCSRFInvalid         = errors.New("csrf token invalid")
	ErrRateLimitExceeded   = errors.New("rate limit exceeded")
)
