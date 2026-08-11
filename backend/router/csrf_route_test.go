package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"coffee-reel/usecase"

	"github.com/labstack/echo/v4"
)

func (s *controllerStub) CSRF(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (s *userUsecaseStub) IssueCSRFToken() (usecase.CSRFTokenResult, error) {
	panic("unexpected IssueCSRFToken call")
}

func TestRouterRegistersCSRFRoute(t *testing.T) {
	e := newTestRouter(
		&controllerStub{},
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	found := false
	for _, route := range e.Routes() {
		if route.Method == http.MethodGet && route.Path == "/csrf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("GET /csrf was not registered")
	}

	req := httptest.NewRequest(http.MethodGet, "/csrf", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
