package middleware

import (
	"context"
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

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{name: "standard", header: "Bearer token", want: "token", ok: true},
		{name: "case insensitive scheme", header: "bearer token", want: "token", ok: true},
		{name: "surrounding whitespace", header: "  Bearer   token  ", want: "token", ok: true},
		{name: "empty", header: "", ok: false},
		{name: "missing token", header: "Bearer", ok: false},
		{name: "wrong scheme", header: "Basic token", ok: false},
		{name: "too many fields", header: "Bearer token extra", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := bearerToken(tt.header)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("bearerToken(%q) = (%q, %v), want (%q, %v)", tt.header, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAuthMiddlewareSuccessUsesCurrentDatabaseUserAndTokenVersion(t *testing.T) {
	dbUser := &entity.User{ID: 10, Name: "current", Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 4}
	claims := usecase.AccessTokenClaims{UserID: 10, Role: entity.RoleAdmin, TokenVersion: 4}
	before := time.Now()

	tokens := &tokenServiceMock{parseAccessTokenFunc: func(token string, now time.Time) (usecase.AccessTokenClaims, error) {
		if token != "access-token" {
			t.Fatalf("ParseAccessToken token = %q", token)
		}
		if now.Before(before) || now.After(time.Now().Add(time.Second)) {
			t.Fatalf("ParseAccessToken now = %s", now)
		}
		return claims, nil
	}}
	users := &userUsecaseMock{
		getMeFunc: func(_ context.Context, userID uint64) (*entity.User, error) {
			if userID != 10 {
				t.Fatalf("GetMe userID = %d", userID)
			}
			return dbUser, nil
		},
		validateTokenVersionFunc: func(user *entity.User, tokenVersion uint64) error {
			if user != dbUser || tokenVersion != 4 {
				t.Fatalf("ValidateTokenVersion(%p, %d)", user, tokenVersion)
			}
			return nil
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer access-token")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	nextCalled := false
	handler := NewAuthMiddleware(users, tokens).Authenticate(func(c echo.Context) error {
		nextCalled = true
		stored, ok := c.Get(userContextKey).(*entity.User)
		if !ok || stored != dbUser {
			t.Fatalf("context user = %#v, want DB user", c.Get(userContextKey))
		}
		if stored.Role != entity.RoleUser {
			t.Fatal("middleware trusted JWT role instead of current DB role")
		}
		return c.NoContent(http.StatusNoContent)
	})

	if err := handler(c); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !nextCalled || rec.Code != http.StatusNoContent {
		t.Fatalf("nextCalled=%v status=%d", nextCalled, rec.Code)
	}
}

func TestAuthMiddlewareRejectsInvalidAuthenticationBeforeController(t *testing.T) {
	internalErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		header     string
		tokens     *tokenServiceMock
		users      *userUsecaseMock
		wantStatus int
	}{
		{name: "missing authorization", header: "", tokens: &tokenServiceMock{}, users: &userUsecaseMock{}, wantStatus: http.StatusUnauthorized},
		{name: "malformed bearer", header: "Basic token", tokens: &tokenServiceMock{}, users: &userUsecaseMock{}, wantStatus: http.StatusUnauthorized},
		{
			name:   "invalid JWT",
			header: "Bearer token",
			tokens: &tokenServiceMock{parseAccessTokenFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
				return usecase.AccessTokenClaims{}, entity.ErrUnauthorized
			}},
			users: &userUsecaseMock{}, wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "missing user",
			header: "Bearer token",
			tokens: &tokenServiceMock{parseAccessTokenFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
				return usecase.AccessTokenClaims{UserID: 1}, nil
			}},
			users:      &userUsecaseMock{getMeFunc: func(context.Context, uint64) (*entity.User, error) { return nil, entity.ErrUnauthorized }},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "suspended user",
			header: "Bearer token",
			tokens: &tokenServiceMock{parseAccessTokenFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
				return usecase.AccessTokenClaims{UserID: 1}, nil
			}},
			users:      &userUsecaseMock{getMeFunc: func(context.Context, uint64) (*entity.User, error) { return nil, entity.ErrUserSuspended }},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "database failure",
			header: "Bearer token",
			tokens: &tokenServiceMock{parseAccessTokenFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
				return usecase.AccessTokenClaims{UserID: 1}, nil
			}},
			users:      &userUsecaseMock{getMeFunc: func(context.Context, uint64) (*entity.User, error) { return nil, internalErr }},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:   "token version mismatch",
			header: "Bearer token",
			tokens: &tokenServiceMock{parseAccessTokenFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
				return usecase.AccessTokenClaims{UserID: 1, TokenVersion: 1}, nil
			}},
			users: &userUsecaseMock{
				getMeFunc: func(context.Context, uint64) (*entity.User, error) {
					return &entity.User{ID: 1, Status: entity.StatusActive, TokenVersion: 2}, nil
				},
				validateTokenVersionFunc: func(*entity.User, uint64) error { return entity.ErrUnauthorized },
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			req.Header.Set(echo.HeaderAuthorization, tt.header)
			req.Header.Set(echo.HeaderXRequestID, "request-123")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			nextCalled := false

			err := NewAuthMiddleware(tt.users, tt.tokens).Authenticate(func(echo.Context) error {
				nextCalled = true
				return nil
			})(c)
			if err != nil {
				t.Fatalf("Authenticate() returned Echo error = %v", err)
			}
			if nextCalled {
				t.Fatal("controller was called for invalid authentication")
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"request_id":"request-123"`) {
				t.Fatalf("error response is missing request ID: %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), internalErr.Error()) {
				t.Fatalf("internal error leaked: %s", rec.Body.String())
			}
		})
	}
}
func TestOptionalAuthAllowsGuest(t *testing.T) {
	tokens := &tokenServiceMock{
		parseAccessTokenFunc: func(
			string,
			time.Time,
		) (usecase.AccessTokenClaims, error) {
			t.Fatal("ParseAccessToken was called for guest request")
			return usecase.AccessTokenClaims{}, nil
		},
	}
	users := &userUsecaseMock{
		getMeFunc: func(
			context.Context,
			uint64,
		) (*entity.User, error) {
			t.Fatal("GetMe was called for guest request")
			return nil, nil
		},
		validateTokenVersionFunc: func(
			*entity.User,
			uint64,
		) error {
			t.Fatal("ValidateTokenVersion was called for guest request")
			return nil
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/videos", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	nextCalled := false

	handler := NewAuthMiddleware(
		users,
		tokens,
	).OptionalAuthenticate(
		func(c echo.Context) error {
			nextCalled = true

			if c.Get(userContextKey) != nil {
				t.Fatalf(
					"context user = %#v, want nil",
					c.Get(userContextKey),
				)
			}

			return c.NoContent(http.StatusNoContent)
		},
	)

	if err := handler(c); err != nil {
		t.Fatalf("OptionalAuthenticate() error = %v", err)
	}
	if !nextCalled {
		t.Fatal("controller was not called")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusNoContent,
		)
	}
}

func TestOptionalAuthAuthenticatesUser(t *testing.T) {
	dbUser := &entity.User{
		ID:           10,
		Name:         "current",
		Role:         entity.RoleUser,
		Status:       entity.StatusActive,
		TokenVersion: 4,
	}

	tokens := &tokenServiceMock{
		parseAccessTokenFunc: func(
			token string,
			now time.Time,
		) (usecase.AccessTokenClaims, error) {
			if token != "access-token" {
				t.Fatalf(
					"token = %q, want access-token",
					token,
				)
			}
			if now.IsZero() {
				t.Fatal("now is zero")
			}

			return usecase.AccessTokenClaims{
				UserID:       dbUser.ID,
				Role:         entity.RoleAdmin,
				TokenVersion: 4,
			}, nil
		},
	}

	users := &userUsecaseMock{
		getMeFunc: func(
			_ context.Context,
			userID uint64,
		) (*entity.User, error) {
			if userID != dbUser.ID {
				t.Fatalf(
					"userID = %d, want %d",
					userID,
					dbUser.ID,
				)
			}

			return dbUser, nil
		},
		validateTokenVersionFunc: func(
			user *entity.User,
			tokenVersion uint64,
		) error {
			if user != dbUser {
				t.Fatal("current DB user was not used")
			}
			if tokenVersion != dbUser.TokenVersion {
				t.Fatalf(
					"tokenVersion = %d, want %d",
					tokenVersion,
					dbUser.TokenVersion,
				)
			}

			return nil
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/videos", nil)
	req.Header.Set(
		echo.HeaderAuthorization,
		"Bearer access-token",
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	nextCalled := false

	handler := NewAuthMiddleware(
		users,
		tokens,
	).OptionalAuthenticate(
		func(c echo.Context) error {
			nextCalled = true

			stored, ok := c.Get(userContextKey).(*entity.User)
			if !ok || stored != dbUser {
				t.Fatalf(
					"context user = %#v, want DB user",
					c.Get(userContextKey),
				)
			}
			if stored.Role != entity.RoleUser {
				t.Fatal("JWT role was trusted instead of DB role")
			}

			return c.NoContent(http.StatusNoContent)
		},
	)

	if err := handler(c); err != nil {
		t.Fatalf("OptionalAuthenticate() error = %v", err)
	}
	if !nextCalled {
		t.Fatal("controller was not called")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusNoContent,
		)
	}
}

func TestOptionalAuthRejectsInvalidHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		tokens *tokenServiceMock
	}{
		{
			name:   "empty",
			header: "",
			tokens: &tokenServiceMock{},
		},
		{
			name:   "whitespace",
			header: "   ",
			tokens: &tokenServiceMock{},
		},
		{
			name:   "missing token",
			header: "Bearer",
			tokens: &tokenServiceMock{},
		},
		{
			name:   "wrong scheme",
			header: "Basic token",
			tokens: &tokenServiceMock{},
		},
		{
			name:   "invalid token",
			header: "Bearer invalid-token",
			tokens: &tokenServiceMock{
				parseAccessTokenFunc: func(
					string,
					time.Time,
				) (usecase.AccessTokenClaims, error) {
					return usecase.AccessTokenClaims{},
						entity.ErrUnauthorized
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(
				http.MethodGet,
				"/videos",
				nil,
			)
			req.Header.Set(
				echo.HeaderAuthorization,
				tt.header,
			)
			req.Header.Set(
				echo.HeaderXRequestID,
				"request-123",
			)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			nextCalled := false

			handler := NewAuthMiddleware(
				&userUsecaseMock{},
				tt.tokens,
			).OptionalAuthenticate(
				func(echo.Context) error {
					nextCalled = true
					return nil
				},
			)

			if err := handler(c); err != nil {
				t.Fatalf(
					"OptionalAuthenticate() error = %v",
					err,
				)
			}
			if nextCalled {
				t.Fatal("controller was called")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf(
					"status = %d, want %d",
					rec.Code,
					http.StatusUnauthorized,
				)
			}
			if !strings.Contains(
				rec.Body.String(),
				`"code":"unauthorized"`,
			) {
				t.Fatalf(
					"response = %s, want unauthorized",
					rec.Body.String(),
				)
			}
			if !strings.Contains(
				rec.Body.String(),
				`"request_id":"request-123"`,
			) {
				t.Fatalf(
					"request ID is missing: %s",
					rec.Body.String(),
				)
			}
		})
	}
}
