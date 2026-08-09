package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"coffee-reel/controller"

	"github.com/labstack/echo/v4"
)

type healthControllerStub struct{}

func (s *healthControllerStub) Check(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

var _ controller.IHealthController = (*healthControllerStub)(nil)

func TestRouterRegistersHealthRoute(t *testing.T) {
	e := newTestRouter(
		&controllerStub{},
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	want := map[string]bool{
		"GET /health": false,
	}
	assertRoutes(t, e, want)
}

func TestHealthRouteDoesNotRequireAuthentication(t *testing.T) {
	e := newTestRouter(
		&controllerStub{},
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
