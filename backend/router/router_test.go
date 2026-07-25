package router

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"coffee-reel/controller"
	"coffee-reel/entity"
	appmiddleware "coffee-reel/middleware"
	"coffee-reel/usecase"

	"github.com/labstack/echo/v4"
)

type controllerStub struct {
	signUpFunc  func(echo.Context) error
	loginFunc   func(echo.Context) error
	refreshFunc func(echo.Context) error
	logoutFunc  func(echo.Context) error
	meFunc      func(echo.Context) error
}

func (s *controllerStub) SignUp(c echo.Context) error {
	if s.signUpFunc == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return s.signUpFunc(c)
}
func (s *controllerStub) Login(c echo.Context) error {
	if s.loginFunc == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return s.loginFunc(c)
}
func (s *controllerStub) Refresh(c echo.Context) error {
	if s.refreshFunc == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return s.refreshFunc(c)
}
func (s *controllerStub) Logout(c echo.Context) error {
	if s.logoutFunc == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return s.logoutFunc(c)
}
func (s *controllerStub) Me(c echo.Context) error {
	if s.meFunc == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return s.meFunc(c)
}

type userUsecaseStub struct {
	getMeFunc                func(context.Context, uint64) (*entity.User, error)
	validateTokenVersionFunc func(*entity.User, uint64) error
}

func (s *userUsecaseStub) SignUp(context.Context, string, string, string) (*entity.User, error) {
	panic("unexpected SignUp call")
}
func (s *userUsecaseStub) Login(context.Context, string, string) (usecase.LoginResult, error) {
	panic("unexpected Login call")
}
func (s *userUsecaseStub) Refresh(context.Context, string) (usecase.AuthTokens, error) {
	panic("unexpected Refresh call")
}
func (s *userUsecaseStub) Logout(context.Context, string) error { panic("unexpected Logout call") }
func (s *userUsecaseStub) GetMe(ctx context.Context, userID uint64) (*entity.User, error) {
	if s.getMeFunc == nil {
		panic("unexpected GetMe call")
	}
	return s.getMeFunc(ctx, userID)
}
func (s *userUsecaseStub) ValidateTokenVersion(user *entity.User, version uint64) error {
	if s.validateTokenVersionFunc == nil {
		panic("unexpected ValidateTokenVersion call")
	}
	return s.validateTokenVersionFunc(user, version)
}

type tokenServiceStub struct {
	parseFunc   func(string, time.Time) (usecase.AccessTokenClaims, error)
	compareFunc func(string, string) error
}

func (s *tokenServiceStub) HashPassword(string) (string, error)  { panic("unexpected") }
func (s *tokenServiceStub) ComparePassword(string, string) error { panic("unexpected") }
func (s *tokenServiceStub) GenerateAccessToken(uint64, entity.UserRole, uint64, time.Time) (string, error) {
	panic("unexpected")
}
func (s *tokenServiceStub) ParseAccessToken(token string, now time.Time) (usecase.AccessTokenClaims, error) {
	if s.parseFunc == nil {
		return usecase.AccessTokenClaims{}, entity.ErrUnauthorized
	}
	return s.parseFunc(token, now)
}
func (s *tokenServiceStub) GenerateRefreshToken() (string, error) { panic("unexpected") }
func (s *tokenServiceStub) HashRefreshToken(string) string        { panic("unexpected") }
func (s *tokenServiceStub) GenerateFamilyID() (string, error)     { panic("unexpected") }
func (s *tokenServiceStub) GenerateCSRFToken() (string, error)    { panic("unexpected") }
func (s *tokenServiceStub) CompareCSRFToken(cookie, header string) error {
	if s.compareFunc == nil {
		return entity.ErrCSRFInvalid
	}
	return s.compareFunc(cookie, header)
}

type rateLimitUsecaseStub struct{}

func (s *rateLimitUsecaseStub) AllowSignup(context.Context, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}
func (s *rateLimitUsecaseStub) AllowLoginIP(context.Context, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}
func (s *rateLimitUsecaseStub) AllowLoginEmail(context.Context, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}
func (s *rateLimitUsecaseStub) AllowRefresh(context.Context, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}

func newTestRouter(userController controller.IUserController, users *userUsecaseStub, tokens *tokenServiceStub) *echo.Echo {
	rateLimits := &rateLimitUsecaseStub{}
	return NewRouter(
		userController,
		appmiddleware.NewAuthMiddleware(users, tokens),
		appmiddleware.NewCSRFMiddleware(tokens),
		appmiddleware.NewRateLimitMiddleware(rateLimits),
		"http://localhost:3000",
		AdminComponents{
			Controller: &adminControllerStub{},
			Middleware: appmiddleware.NewAdminMiddleware(),
		},
	)
}

func TestRouterRegistersOnlyTheSpecifiedAuthenticationRoutes(t *testing.T) {
	e := newTestRouter(&controllerStub{}, &userUsecaseStub{}, &tokenServiceStub{})
	want := map[string]string{
		"POST /signup":  "",
		"POST /login":   "",
		"POST /refresh": "",
		"POST /logout":  "",
		"GET /me":       "",
	}
	for _, route := range e.Routes() {
		key := route.Method + " " + route.Path
		if _, exists := want[key]; exists {
			want[key] = route.Name
		}
	}
	for route, name := range want {
		if name == "" {
			t.Fatalf("route %s is not registered", route)
		}
	}
}

func TestRouterAppliesSecurityHeadersCORSRequestIDAndBodyLimit(t *testing.T) {
	controllerCalled := false
	controllers := &controllerStub{signUpFunc: func(c echo.Context) error {
		controllerCalled = true
		return c.NoContent(http.StatusNoContent)
	}}
	e := newTestRouter(controllers, &userUsecaseStub{}, &tokenServiceStub{})

	t.Run("security headers and allowed CORS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{"name":"a"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderOrigin, "http://localhost:3000")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent || !controllerCalled {
			t.Fatalf("status=%d controllerCalled=%v body=%s", rec.Code, controllerCalled, rec.Body.String())
		}
		if rec.Header().Get(echo.HeaderXRequestID) == "" {
			t.Fatal("Request ID middleware did not set a response request ID")
		}
		if rec.Header().Get(echo.HeaderAccessControlAllowOrigin) != "http://localhost:3000" || rec.Header().Get(echo.HeaderAccessControlAllowCredentials) != "true" {
			t.Fatalf("CORS headers = %v", rec.Header())
		}
		if rec.Header().Get(echo.HeaderXContentTypeOptions) != "nosniff" || rec.Header().Get(echo.HeaderXFrameOptions) != "DENY" {
			t.Fatalf("security headers = %v", rec.Header())
		}
		if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatalf("CSP = %q", rec.Header().Get("Content-Security-Policy"))
		}
	})

	t.Run("unapproved origin gets no browser permission", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{"name":"a"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderOrigin, "https://evil.example")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Header().Get(echo.HeaderAccessControlAllowOrigin) != "" {
			t.Fatalf("unapproved origin was allowed: %q", rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
		}
	})

	t.Run("oversized body never reaches controller", func(t *testing.T) {
		controllerCalled = false
		req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(strings.Repeat("x", 65537)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want 413, body=%s", rec.Code, rec.Body.String())
		}
		if controllerCalled {
			t.Fatal("controller received an oversized body")
		}
	})
}

func TestRouterRefreshRequiresRateLimitThenCSRFBeforeController(t *testing.T) {
	controllerCalled := false
	controllers := &controllerStub{refreshFunc: func(c echo.Context) error {
		controllerCalled = true
		return c.NoContent(http.StatusNoContent)
	}}
	tokens := &tokenServiceStub{compareFunc: func(cookie, header string) error {
		if cookie != header {
			return entity.ErrCSRFInvalid
		}
		return nil
	}}
	e := newTestRouter(controllers, &userUsecaseStub{}, tokens)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "refresh"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "csrf"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || controllerCalled {
		t.Fatalf("missing CSRF header status=%d controllerCalled=%v body=%s", rec.Code, controllerCalled, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "refresh"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "csrf"})
	req.Header.Set(echo.HeaderXCSRFToken, "csrf")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !controllerCalled {
		t.Fatalf("valid refresh status=%d controllerCalled=%v body=%s", rec.Code, controllerCalled, rec.Body.String())
	}
}

func TestRouterAuthFlowDoesNotLogAuthorizationOrCookieSecrets(t *testing.T) {
	user := &entity.User{ID: 1, Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 2}
	users := &userUsecaseStub{
		getMeFunc:                func(context.Context, uint64) (*entity.User, error) { return user, nil },
		validateTokenVersionFunc: func(*entity.User, uint64) error { return nil },
	}
	tokens := &tokenServiceStub{parseFunc: func(token string, _ time.Time) (usecase.AccessTokenClaims, error) {
		if token != "authorization-secret" {
			return usecase.AccessTokenClaims{}, errors.New("unexpected token")
		}
		return usecase.AccessTokenClaims{UserID: 1, TokenVersion: 2}, nil
	}}
	controllers := &controllerStub{meFunc: func(c echo.Context) error { return c.NoContent(http.StatusNoContent) }}
	e := newTestRouter(controllers, users, tokens)
	var logs bytes.Buffer
	e.Logger.SetOutput(&logs)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer authorization-secret")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "cookie-secret"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), "authorization-secret") || strings.Contains(logs.String(), "cookie-secret") {
		t.Fatalf("logger exposed authentication secrets: %s", logs.String())
	}
}

func TestRouterRecoverConvertsPanicWithoutReturningStackTrace(t *testing.T) {
	controllers := &controllerStub{signUpFunc: func(echo.Context) error { panic("secret panic stack detail") }}
	e := newTestRouter(controllers, &userUsecaseStub{}, &tokenServiceStub{})
	req := httptest.NewRequest(http.MethodPost, "/signup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret panic stack detail") || strings.Contains(rec.Body.String(), "goroutine") {
		t.Fatalf("panic or stack trace leaked: %s", rec.Body.String())
	}
}
