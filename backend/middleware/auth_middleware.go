package middleware

import (
	"coffee-reel/entity"
	"coffee-reel/usecase"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const userContextKey = "user"

type AuthMiddleware struct {
	users  usecase.IUserUsecase
	tokens usecase.ITokenService
}

type middlewareAPIErrorResponse struct {
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func NewAuthMiddleware(users usecase.IUserUsecase, tokens usecase.ITokenService) *AuthMiddleware {
	return &AuthMiddleware{users: users, tokens: tokens}
}

// Access Token、User状態、TokenVersionを確認し、認証済みUserをEcho Contextへ保存する。
func (m *AuthMiddleware) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tokenString, ok := bearerToken(c.Request().Header.Get("Authorization"))
		if !ok {
			return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
		}

		claims, err := m.tokens.ParseAccessToken(tokenString, time.Now())
		if err != nil {
			return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
		}

		// JWT内のRoleやStatusだけを信用せず、DB上の現在Userを再取得する。
		user, err := m.users.GetMe(c.Request().Context(), claims.UserID)
		if err != nil {
			switch {
			case errors.Is(err, entity.ErrUnauthorized),
				errors.Is(err, entity.ErrUserNotFound),
				errors.Is(err, entity.ErrUserSuspended):

				return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")

			default:
				return writeMiddlewareError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
			}
		}

		if err := m.users.ValidateTokenVersion(user, claims.TokenVersion); err != nil {
			return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
		}

		c.Set(userContextKey, user)

		return next(c)
	}
}

// OptionalAuthenticateはAuthorization Headerがない場合だけGuestとして後続処理へ進める。
// Headerが存在する場合は通常認証を行い、不正なTokenをGuestへ降格させない。
func (m *AuthMiddleware) OptionalAuthenticate(next echo.HandlerFunc) echo.HandlerFunc {
	authenticated := m.Authenticate(next)

	return func(c echo.Context) error {
		if _, exists := c.Request().Header[echo.HeaderAuthorization]; !exists {
			return next(c)
		}

		return authenticated(c)
	}
}

// Authorization HeaderからBearer Tokenを取り出す
func bearerToken(authorization string) (string, bool) {
	parts := strings.Fields(authorization)

	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	if parts[1] == "" {
		return "", false
	}

	return parts[1], true
}
