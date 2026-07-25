package middleware

import (
	"coffee-reel/entity"
	"net/http"

	"github.com/labstack/echo/v4"
)

type AdminMiddleware struct{}

func NewAdminMiddleware() *AdminMiddleware {
	return &AdminMiddleware{}
}

func (m *AdminMiddleware) Authorize(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, ok := c.Get(userContextKey).(*entity.User)
		if !ok || user == nil || !user.IsActive() {
			return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
		}
		if !user.IsAdmin() {
			c.Logger().Warnf(
				"admin access denied request_id=%s user_id=%d",
				middlewareRequestID(c),
				user.ID,
			)
			return writeMiddlewareError(c, http.StatusForbidden, "admin_required", "管理者権限が必要です")
		}

		return next(c)
	}
}
