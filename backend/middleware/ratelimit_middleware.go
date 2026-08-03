package middleware

import (
	"context"

	"coffee-reel/entity"
	"coffee-reel/usecase"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const refreshCookieName = "refresh_token"

type RateLimitMiddleware struct {
	rateLimits usecase.IRateLimitUsecase
}

type apiErrorResponse struct {
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeMiddlewareError(c echo.Context, status int, code string, message string) error {
	return c.JSON(status, apiErrorResponse{
		Status:    status,
		Code:      code,
		Message:   message,
		RequestID: middlewareRequestID(c),
	})
}

func middlewareRequestID(c echo.Context) string {
	if id := c.Response().Header().Get(echo.HeaderXRequestID); id != "" {
		return id
	}
	return c.Request().Header.Get(echo.HeaderXRequestID)
}

func NewRateLimitMiddleware(rateLimits usecase.IRateLimitUsecase) *RateLimitMiddleware {
	return &RateLimitMiddleware{rateLimits: rateLimits}
}

func (m *RateLimitMiddleware) Signup(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ip, ok := clientIP(c)
		if !ok {
			return writeMiddlewareError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
		}

		decision, err := m.rateLimits.AllowSignup(c.Request().Context(), ip)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}
		return applyRateLimitDecision(c, next, decision)
	}
}

func (m *RateLimitMiddleware) LoginIP(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ip, ok := clientIP(c)
		if !ok {
			return writeMiddlewareError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
		}

		decision, err := m.rateLimits.AllowLoginIP(c.Request().Context(), ip)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}
		return applyRateLimitDecision(c, next, decision)
	}
}

func (m *RateLimitMiddleware) Refresh(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(refreshCookieName)
		if err != nil || cookie.Value == "" {
			return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
		}

		decision, err := m.rateLimits.AllowRefresh(c.Request().Context(), cookie.Value)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}
		return applyRateLimitDecision(c, next, decision)
	}
}

func (m *RateLimitMiddleware) VideoStart(next echo.HandlerFunc) echo.HandlerFunc {
	return m.video(next, m.rateLimits.AllowVideoStartUser, m.rateLimits.AllowVideoStartIP)
}

func (m *RateLimitMiddleware) VideoComplete(next echo.HandlerFunc) echo.HandlerFunc {
	return m.video(next, m.rateLimits.AllowVideoCompleteUser, m.rateLimits.AllowVideoCompleteIP)
}

func (m *RateLimitMiddleware) video(
	next echo.HandlerFunc,
	allowUser func(context.Context, uint64) (usecase.RateLimitDecision, error),
	allowIP func(context.Context, string) (usecase.RateLimitDecision, error),
) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, ok := c.Get(userContextKey).(*entity.User)
		if !ok || user == nil || user.ID == 0 {
			return writeMiddlewareError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
		}

		ctx := c.Request().Context()
		decision, err := allowUser(ctx, user.ID)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}
		if !decision.Allowed {
			return rejectRateLimit(c, decision)
		}

		ip, ok := clientIP(c)
		if !ok {
			return writeMiddlewareError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
		}
		decision, err = allowIP(ctx, ip)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}
		return applyRateLimitDecision(c, next, decision)
	}
}

func applyRateLimitDecision(c echo.Context, next echo.HandlerFunc, decision usecase.RateLimitDecision) error {
	if decision.Allowed {
		return next(c)
	}
	return rejectRateLimit(c, decision)
}

func rejectRateLimit(c echo.Context, decision usecase.RateLimitDecision) error {
	c.Response().Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(decision.RetryAfter), 10))
	return writeMiddlewareError(c, http.StatusTooManyRequests, "rate_limit_exceeded", "リクエスト回数が上限を超えました")
}

func clientIP(c echo.Context) (string, bool) {
	value := strings.TrimSpace(c.RealIP())
	if value == "" {
		return "", false
	}

	ip, err := netip.ParseAddr(value)
	if err != nil {
		return "", false
	}
	return ip.Unmap().String(), true
}

func retryAfterSeconds(value time.Duration) int64 {
	if value <= 0 {
		return 1
	}
	return int64((value + time.Second - 1) / time.Second)
}
