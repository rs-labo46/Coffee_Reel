package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type healthUsecaseMock struct {
	checkFunc func(context.Context) error
}

func (m *healthUsecaseMock) Check(ctx context.Context) error {
	if m.checkFunc == nil {
		panic("unexpected HealthUsecase.Check call")
	}
	return m.checkFunc(ctx)
}

func TestHealthControllerCheckReturnsOK(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	controller := NewHealthController(&healthUsecaseMock{
		checkFunc: func(context.Context) error {
			return nil
		},
	})

	if err := controller.Check(ctx); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %s", got)
	}
}

func TestHealthControllerCheckReturnsUnavailable(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	controller := NewHealthController(&healthUsecaseMock{
		checkFunc: func(context.Context) error {
			return errors.New("dependency unavailable")
		},
	})

	if err := controller.Check(ctx); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"unavailable"}` {
		t.Fatalf("body = %s", got)
	}
}
