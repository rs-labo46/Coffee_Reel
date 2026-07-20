package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/usecase"

	"github.com/labstack/echo/v4"
)

func newControllerContext(method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderXRequestID, "request-controller")
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func cookieByName(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q was not set: %v", name, rec.Header().Values(echo.HeaderSetCookie))
	return nil
}

func TestUserControllerSignUpValidatesThenReturnsSafeCreatedUser(t *testing.T) {
	createdAt := time.Date(2026, 7, 21, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	validatorCalled := false
	usecaseCalled := false
	validator := &userValidatorMock{validateSignupFunc: func(name, email, password string) (string, string, string, error) {
		validatorCalled = true
		if name != " Raw Name " || email != " USER@EXAMPLE.COM " || password != " password " {
			t.Fatalf("ValidateSignup(%q, %q, %q)", name, email, password)
		}
		return "Raw Name", "user@example.com", " password ", nil
	}}
	users := &userUsecaseMock{signUpFunc: func(_ context.Context, name, email, password string) (*entity.User, error) {
		usecaseCalled = true
		if name != "Raw Name" || email != "user@example.com" || password != " password " {
			t.Fatalf("SignUp(%q, %q, %q)", name, email, password)
		}
		return &entity.User{ID: 10, Name: name, Email: email, PasswordHash: "secret-hash", Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 99, CreatedAt: createdAt}, nil
	}}
	controller := NewUserController(users, &rateLimitUsecaseMock{}, validator, CookieConfig{})
	c, rec := newControllerContext(http.MethodPost, "/signup", `{"name":" Raw Name ","email":" USER@EXAMPLE.COM ","password":" password "}`)

	if err := controller.SignUp(c); err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	if !validatorCalled || !usecaseCalled || rec.Code != http.StatusCreated {
		t.Fatalf("validator=%v usecase=%v status=%d", validatorCalled, usecaseCalled, rec.Code)
	}
	var response userDataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body=%s", err, rec.Body.String())
	}
	if response.Data.ID != 10 || response.Data.Email != "user@example.com" || !response.Data.CreatedAt.Equal(createdAt.UTC()) {
		t.Fatalf("response = %+v", response)
	}
	for _, secret := range []string{"secret-hash", "password", "token_version", "99"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("signup response leaks %q: %s", secret, rec.Body.String())
		}
	}
}

func TestUserControllerSignUpRejectsMalformedJSONBeforeValidation(t *testing.T) {
	controller := NewUserController(&userUsecaseMock{}, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{})
	c, rec := newControllerContext(http.MethodPost, "/signup", `{"name":`)

	if err := controller.SignUp(c); err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var response apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Code != "validation_failed" || response.RequestID != "request-controller" {
		t.Fatalf("response = %+v", response)
	}
}

func TestUserControllerLoginSuccessSetsSecureCookieContractWithoutReturningRefreshSecrets(t *testing.T) {
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second)
	validator := &userValidatorMock{validateLoginFunc: func(email, password string) (string, string, error) {
		if email != " USER@EXAMPLE.COM " || password != " password " {
			t.Fatalf("ValidateLogin(%q, %q)", email, password)
		}
		return "user@example.com", " password ", nil
	}}
	rateLimits := &rateLimitUsecaseMock{allowLoginEmailFunc: func(_ context.Context, email string) (usecase.RateLimitDecision, error) {
		if email != "user@example.com" {
			t.Fatalf("AllowLoginEmail email = %q", email)
		}
		return usecase.RateLimitDecision{Allowed: true}, nil
	}}
	users := &userUsecaseMock{loginFunc: func(_ context.Context, email, password string) (usecase.LoginResult, error) {
		if email != "user@example.com" || password != " password " {
			t.Fatalf("Login(%q, %q)", email, password)
		}
		return usecase.LoginResult{
			User:       &entity.User{ID: 1, Name: "Alice", Email: email, PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 8},
			AuthTokens: usecase.AuthTokens{AccessToken: "access-token", RefreshToken: "refresh-secret", CSRFToken: "csrf-secret", RefreshTokenExpiresAt: expiresAt},
		}, nil
	}}
	controller := NewUserController(users, rateLimits, validator, CookieConfig{Secure: true, CSRFDomain: "example.com"})
	c, rec := newControllerContext(http.MethodPost, "/login", `{"email":" USER@EXAMPLE.COM ","password":" password "}`)

	if err := controller.Login(c); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response authResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Data.AccessToken != "access-token" || response.Data.TokenType != "Bearer" || response.Data.ExpiresIn != 900 {
		t.Fatalf("response = %+v", response)
	}
	if response.Data.User.Role != entity.RoleUser || response.Data.User.Status != entity.StatusActive {
		t.Fatalf("user response = %+v", response.Data.User)
	}
	for _, secret := range []string{"refresh-secret", "csrf-secret", "PasswordHash", "token_version"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("login JSON leaks %q: %s", secret, rec.Body.String())
		}
	}

	refreshCookie := cookieByName(t, rec, refreshCookieName)
	if refreshCookie.Value != "refresh-secret" || !refreshCookie.HttpOnly || !refreshCookie.Secure || refreshCookie.Path != "/" || refreshCookie.SameSite != http.SameSiteLaxMode || refreshCookie.MaxAge != cookieMaxAge || !refreshCookie.Expires.Equal(expiresAt) {
		t.Fatalf("refresh cookie = %+v", refreshCookie)
	}
	csrfCookie := cookieByName(t, rec, csrfCookieName)
	if csrfCookie.Value != "csrf-secret" || csrfCookie.HttpOnly || !csrfCookie.Secure || csrfCookie.Path != "/" || csrfCookie.Domain != "example.com" || csrfCookie.SameSite != http.SameSiteLaxMode || csrfCookie.MaxAge != cookieMaxAge || !csrfCookie.Expires.Equal(expiresAt) {
		t.Fatalf("csrf cookie = %+v", csrfCookie)
	}
}

func TestUserControllerLoginEmailRateLimitBlocksAuthentication(t *testing.T) {
	loginCalled := false
	users := &userUsecaseMock{loginFunc: func(context.Context, string, string) (usecase.LoginResult, error) {
		loginCalled = true
		return usecase.LoginResult{}, nil
	}}
	validator := &userValidatorMock{validateLoginFunc: func(string, string) (string, string, error) {
		return "user@example.com", "password", nil
	}}
	rateLimits := &rateLimitUsecaseMock{allowLoginEmailFunc: func(context.Context, string) (usecase.RateLimitDecision, error) {
		return usecase.RateLimitDecision{Allowed: false, RetryAfter: 1201 * time.Millisecond}, nil
	}}
	controller := NewUserController(users, rateLimits, validator, CookieConfig{})
	c, rec := newControllerContext(http.MethodPost, "/login", `{"email":"user@example.com","password":"password"}`)

	if err := controller.Login(c); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginCalled {
		t.Fatal("UserUsecase.Login was called after email rate limit denial")
	}
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("status=%d Retry-After=%q body=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
}

func TestUserControllerLoginRateLimitStorageFailureReturnsGeneric503(t *testing.T) {
	redisErr := errors.New("redis connection contains internal details")
	validator := &userValidatorMock{validateLoginFunc: func(string, string) (string, string, error) { return "user@example.com", "password", nil }}
	rateLimits := &rateLimitUsecaseMock{allowLoginEmailFunc: func(context.Context, string) (usecase.RateLimitDecision, error) {
		return usecase.RateLimitDecision{}, redisErr
	}}
	controller := NewUserController(&userUsecaseMock{}, rateLimits, validator, CookieConfig{})
	c, rec := newControllerContext(http.MethodPost, "/login", `{"email":"user@example.com","password":"password"}`)

	if err := controller.Login(c); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), redisErr.Error()) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserControllerRefreshSuccessRotatesBothCookies(t *testing.T) {
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second)
	users := &userUsecaseMock{refreshFunc: func(_ context.Context, token string) (usecase.AuthTokens, error) {
		if token != "old-refresh" {
			t.Fatalf("Refresh token = %q", token)
		}
		return usecase.AuthTokens{AccessToken: "new-access", RefreshToken: "new-refresh", CSRFToken: "new-csrf", RefreshTokenExpiresAt: expiresAt}, nil
	}}
	controller := NewUserController(users, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{Secure: true, CSRFDomain: "example.com"})
	c, rec := newControllerContext(http.MethodPost, "/refresh", "")
	c.Request().AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-refresh"})

	if err := controller.Refresh(c); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response refreshResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Data.AccessToken != "new-access" || response.Data.TokenType != "Bearer" || response.Data.ExpiresIn != 900 {
		t.Fatalf("response = %+v", response)
	}
	if strings.Contains(rec.Body.String(), "new-refresh") || strings.Contains(rec.Body.String(), "new-csrf") {
		t.Fatalf("refresh response leaks cookie token: %s", rec.Body.String())
	}
	if cookieByName(t, rec, refreshCookieName).Value != "new-refresh" || cookieByName(t, rec, csrfCookieName).Value != "new-csrf" {
		t.Fatal("refresh did not rotate both authentication cookies")
	}
}

func TestUserControllerRefreshReuseClearsCookiesAndReturnsGenericUnauthorized(t *testing.T) {
	users := &userUsecaseMock{refreshFunc: func(context.Context, string) (usecase.AuthTokens, error) {
		return usecase.AuthTokens{}, entity.ErrRefreshTokenReused
	}}
	controller := NewUserController(users, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{Secure: true, CSRFDomain: "example.com"})
	c, rec := newControllerContext(http.MethodPost, "/refresh", "")
	c.Request().AddCookie(&http.Cookie{Name: refreshCookieName, Value: "reused-secret"})

	if err := controller.Refresh(c); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "reused") {
		t.Fatalf("reuse details or token leaked: %s", rec.Body.String())
	}
	for _, name := range []string{refreshCookieName, csrfCookieName} {
		cookie := cookieByName(t, rec, name)
		if cookie.Value != "" || cookie.MaxAge != -1 || cookie.Expires.After(time.Unix(1, 0)) {
			t.Fatalf("cleared %s cookie = %+v", name, cookie)
		}
	}
}

func TestUserControllerRefreshMissingCookieDoesNotCallUsecase(t *testing.T) {
	controller := NewUserController(&userUsecaseMock{}, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{})
	c, rec := newControllerContext(http.MethodPost, "/refresh", "")

	if err := controller.Refresh(c); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserControllerLogoutPassesCookieAndClearsBothCookies(t *testing.T) {
	logoutToken := ""
	users := &userUsecaseMock{logoutFunc: func(_ context.Context, token string) error {
		logoutToken = token
		return nil
	}}
	controller := NewUserController(users, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{Secure: true, CSRFDomain: "example.com"})
	c, rec := newControllerContext(http.MethodPost, "/logout", "")
	c.Request().AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-secret"})

	if err := controller.Logout(c); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if logoutToken != "refresh-secret" || rec.Code != http.StatusNoContent {
		t.Fatalf("logoutToken=%q status=%d", logoutToken, rec.Code)
	}
	for _, name := range []string{refreshCookieName, csrfCookieName} {
		cookie := cookieByName(t, rec, name)
		if cookie.MaxAge != -1 || cookie.Value != "" || !cookie.Secure {
			t.Fatalf("cleared %s cookie = %+v", name, cookie)
		}
	}
}

func TestUserControllerLogoutWithoutRefreshCookieIsIdempotent(t *testing.T) {
	called := false
	users := &userUsecaseMock{logoutFunc: func(_ context.Context, token string) error {
		called = true
		if token != "" {
			t.Fatalf("Logout token = %q, want empty", token)
		}
		return nil
	}}
	controller := NewUserController(users, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{})
	c, rec := newControllerContext(http.MethodPost, "/logout", "")
	if err := controller.Logout(c); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}

func TestUserControllerMeRequiresMiddlewareUserAndReturnsSafeResponse(t *testing.T) {
	controller := NewUserController(&userUsecaseMock{}, &rateLimitUsecaseMock{}, &userValidatorMock{}, CookieConfig{})

	t.Run("missing context user", func(t *testing.T) {
		c, rec := newControllerContext(http.MethodGet, "/me", "")
		if err := controller.Me(c); err != nil {
			t.Fatalf("Me() error = %v", err)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("current user", func(t *testing.T) {
		c, rec := newControllerContext(http.MethodGet, "/me", "")
		c.Set(userContextKey, &entity.User{ID: 1, Name: "Alice", Email: "alice@example.com", PasswordHash: "hash", Role: entity.RoleAdmin, Status: entity.StatusActive, TokenVersion: 4, CreatedAt: time.Now().UTC()})
		if err := controller.Me(c); err != nil {
			t.Fatalf("Me() error = %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "hash") || strings.Contains(rec.Body.String(), "token_version") {
			t.Fatalf("Me response leaks auth state: %s", rec.Body.String())
		}
	})
}

func TestWriteErrorMapsDomainErrorsAndNeverLeaksInternalDetails(t *testing.T) {
	internalErr := errors.New("pq: duplicate key constraint secret_table_key stack trace")
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "validation", err: entity.ErrInvalidInput, wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "email duplicate", err: entity.ErrEmailAlreadyExists, wantStatus: http.StatusConflict, wantCode: "email_already_exists"},
		{name: "credentials", err: entity.ErrInvalidCredentials, wantStatus: http.StatusUnauthorized, wantCode: "invalid_credentials"},
		{name: "suspended", err: entity.ErrUserSuspended, wantStatus: http.StatusForbidden, wantCode: "user_suspended"},
		{name: "refresh invalid", err: entity.ErrRefreshTokenInvalid, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "internal", err: internalErr, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newControllerContext(http.MethodGet, "/", "")
			if err := writeError(c, tt.err); err != nil {
				t.Fatalf("writeError() error = %v", err)
			}
			var response apiErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if response.Status != tt.wantStatus || response.Code != tt.wantCode || response.RequestID != "request-controller" {
				t.Fatalf("response = %+v", response)
			}
			if strings.Contains(rec.Body.String(), internalErr.Error()) || strings.Contains(rec.Body.String(), "secret_table_key") || strings.Contains(rec.Body.String(), "stack trace") {
				t.Fatalf("internal details leaked: %s", rec.Body.String())
			}
		})
	}
}
