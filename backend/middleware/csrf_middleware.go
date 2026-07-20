package middleware

import (
	"coffee-reel/usecase"
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	csrfCookieName = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
)

type CSRFMiddleware struct {
	tokens usecase.ITokenService
}

func NewCSRFMiddleware(tokens usecase.ITokenService) *CSRFMiddleware {
	return &CSRFMiddleware{tokens}
}

// CSRF CookieとX-CSRF-Token Headerが一致する場合だけ後続処理を実行する
func (m *CSRFMiddleware) Validate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(csrfCookieName)
		if err != nil {
			return writeMiddlewareError(c, http.StatusForbidden, "csrf_invalid", "CSRFトークンが無効です")
		}
		headerToken := c.Request().Header.Get(csrfHeaderName)

		if err := m.tokens.CompareCSRFToken(cookie.Value, headerToken); err != nil {
			return writeMiddlewareError(c, http.StatusForbidden, "csrf_invalid", "CSRFトークンが無効です")
		}
		return next(c)
	}
}
