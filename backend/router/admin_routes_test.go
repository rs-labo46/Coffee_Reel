package router

import (
	"coffee-reel/controller"
	appmiddleware "coffee-reel/middleware"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

type adminControllerStub struct{}

func (s *adminControllerStub) ListUsers(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}
func (s *adminControllerStub) GetUserDetail(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}
func (s *adminControllerStub) SuspendUser(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}
func (s *adminControllerStub) ResumeUser(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

var _ controller.IAdminUserController = (*adminControllerStub)(nil)

func TestRouterRegistersAdminUserRoutes(t *testing.T) {
	rateLimits := &rateLimitUsecaseStub{}
	e := NewRouter(
		&controllerStub{},
		appmiddleware.NewAuthMiddleware(&userUsecaseStub{}, &tokenServiceStub{}),
		appmiddleware.NewCSRFMiddleware(&tokenServiceStub{}),
		appmiddleware.NewRateLimitMiddleware(rateLimits),
		"http://localhost:3000",
		AdminComponents{
			Controller: &adminControllerStub{},
			Middleware: appmiddleware.NewAdminMiddleware(),
		},
	)

	want := map[string]bool{
		"GET /admin/users":                    false,
		"GET /admin/users/:user_id":           false,
		"PATCH /admin/users/:user_id/suspend": false,
		"PATCH /admin/users/:user_id/resume":  false,
	}
	for _, route := range e.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, registered := range want {
		if !registered {
			t.Fatalf("route %s is not registered", route)
		}
	}
}

func TestRouterAllowsPatchInCORSPreflight(t *testing.T) {
	e := newTestRouter(&controllerStub{}, &userUsecaseStub{}, &tokenServiceStub{})
	req := httptest.NewRequest(http.MethodOptions, "/admin/users/1/suspend", nil)
	req.Header.Set(echo.HeaderOrigin, "http://localhost:3000")
	req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodPatch)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Header().Get(echo.HeaderAccessControlAllowMethods) == "" {
		t.Fatalf("PATCH preflight was not allowed: headers=%v", rec.Header())
	}
}
