package middleware

import (
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
	return c.JSON(status, apiErrorResponse{Status: status, Code: code, Message: message, RequestID: middlewareRequestID(c)})
}

// ResponseまたはRequest HeaderからRequest IDを取得する。
func middlewareRequestID(c echo.Context) string {
	if id := c.Response().Header().Get(echo.HeaderXRequestID); id != "" {
		return id
	}

	return c.Request().Header.Get(echo.HeaderXRequestID)
}

func NewRateLimitMiddleware(rateLimits usecase.IRateLimitUsecase) *RateLimitMiddleware {
	return &RateLimitMiddleware{rateLimits: rateLimits}
}

// Client IP単位で会員登録APIのRate Limitを確認する。
func (m *RateLimitMiddleware) Signup(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		clientIP, ok := clientIP(c)
		if !ok {
			return writeMiddlewareError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
		}

		decision, err := m.rateLimits.AllowSignup(c.Request().Context(), clientIP)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}

		return applyRateLimitDecision(c, next, decision)
	}
}

// LoginのEmail制限はValidatorによる正規化後にControllerからUsecaseへ依頼する。
func (m *RateLimitMiddleware) LoginIP(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		clientIP, ok := clientIP(c)
		if !ok {
			return writeMiddlewareError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
		}

		decision, err := m.rateLimits.AllowLoginIP(c.Request().Context(), clientIP)
		if err != nil {
			return writeMiddlewareError(c, http.StatusServiceUnavailable, "service_unavailable", "一時的にサービスを利用できません")
		}

		return applyRateLimitDecision(c, next, decision)
	}
}

// Refresh Token単位でToken再発行APIのRate Limitを確認する。
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

// Rate Limit判定結果に応じて後続処理または429レスポンスを返す。
func applyRateLimitDecision(c echo.Context, next echo.HandlerFunc, decision usecase.RateLimitDecision) error {
	if decision.Allowed {
		return next(c)
	}

	c.Response().Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(decision.RetryAfter), 10))

	return writeMiddlewareError(c, http.StatusTooManyRequests, "rate_limit_exceeded", "リクエスト回数が上限を超えました")
}

// X-Forwarded-Forを直接読まず、Routerで設定した信頼済みProxy経由のRealIPだけを使用する。
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

// 待機時間を切り上げた秒数へ変換する。
func retryAfterSeconds(value time.Duration) int64 {
	if value <= 0 {
		return 1
	}

	return int64((value + time.Second - 1) / time.Second)
}
