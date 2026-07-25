package middleware

import (
	"bytes"
	"coffee-reel/entity"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAdminMiddlewareAllowsActiveAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(userContextKey, &entity.User{ID: 1, Role: entity.RoleAdmin, Status: entity.StatusActive})

	called := false
	handler := NewAdminMiddleware().Authorize(func(echo.Context) error {
		called = true
		return c.NoContent(http.StatusNoContent)
	})

	if err := handler(c); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}

func TestAdminMiddlewareRejectsUser(t *testing.T) {
	e := echo.New()
	var logs bytes.Buffer
	e.Logger.SetOutput(&logs)
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer secret-token")
	req.Header.Set(echo.HeaderXRequestID, "request-admin")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(userContextKey, &entity.User{ID: 2, Role: entity.RoleUser, Status: entity.StatusActive})

	called := false
	handler := NewAdminMiddleware().Authorize(func(echo.Context) error {
		called = true
		return nil
	})

	if err := handler(c); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if called || rec.Code != http.StatusForbidden {
		t.Fatalf("called=%v status=%d body=%s", called, rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), "secret-token") {
		t.Fatalf("authorization token leaked: %s", logs.String())
	}
}

func TestAdminMiddlewareRejectsMissingOrInactiveUser(t *testing.T) {
	tests := []struct {
		name string
		user *entity.User
	}{
		{name: "missing user"},
		{name: "inactive admin", user: &entity.User{ID: 1, Role: entity.RoleAdmin, Status: entity.StatusSuspended}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.user != nil {
				c.Set(userContextKey, tt.user)
			}

			handler := NewAdminMiddleware().Authorize(func(echo.Context) error {
				t.Fatal("next handler must not be called")
				return nil
			})

			if err := handler(c); err != nil {
				t.Fatalf("Authorize() error = %v", err)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
