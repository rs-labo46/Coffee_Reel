package middleware

import (
	"coffee-reel/usecase"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	csrfCookieName      = "csrf_token"
	csrfHeaderName      = "X-CSRF-Token"
	fetchSiteHeaderName = "Sec-Fetch-Site"
	fetchSiteSameOrigin = "same-origin"
)

type CSRFMiddleware struct {
	tokens usecase.ITokenService
}

func NewCSRFMiddleware(tokens usecase.ITokenService) *CSRFMiddleware {
	return &CSRFMiddleware{tokens}
}

// Vercel FrontendとRender APIはcross-siteになるため、same-originだけFetch Metadataで信頼し、
// それ以外とSec-Fetch-Site未対応BrowserはDouble Submit Cookieで検証する。
func (m *CSRFMiddleware) Validate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		fetchSite := strings.ToLower(
			strings.TrimSpace(
				c.Request().Header.Get(fetchSiteHeaderName),
			),
		)

		if fetchSite == fetchSiteSameOrigin {
			return next(c)
		}

		cookie, err := c.Cookie(csrfCookieName)
		if err != nil {
			return writeMiddlewareError(
				c,
				http.StatusForbidden,
				"csrf_invalid",
				"CSRFトークンが無効です",
			)
		}

		headerToken := c.Request().Header.Get(csrfHeaderName)

		if err := m.tokens.CompareCSRFToken(
			cookie.Value,
			headerToken,
		); err != nil {
			return writeMiddlewareError(
				c,
				http.StatusForbidden,
				"csrf_invalid",
				"CSRFトークンが無効です",
			)
		}

		return next(c)
	}
}
