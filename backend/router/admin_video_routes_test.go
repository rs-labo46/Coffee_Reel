package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"coffee-reel/controller"
	"coffee-reel/entity"
	appmiddleware "coffee-reel/middleware"
	"coffee-reel/usecase"

	"github.com/labstack/echo/v4"
)

type adminVideoControllerStub struct {
	listCalled bool
}

func (s *adminVideoControllerStub) List(c echo.Context) error {
	s.listCalled = true
	return c.NoContent(http.StatusNoContent)
}

func (s *adminVideoControllerStub) Detail(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (s *adminVideoControllerStub) Hide(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (s *adminVideoControllerStub) Restore(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

var _ controller.IAdminVideoController = (*adminVideoControllerStub)(nil)

func newAdminVideoTestRouter(
	videoController controller.IAdminVideoController,
	users *userUsecaseStub,
	tokens *tokenServiceStub,
) *echo.Echo {
	rateLimits := &rateLimitUsecaseStub{}

	return NewRouter(
		&controllerStub{},
		appmiddleware.NewAuthMiddleware(users, tokens),
		appmiddleware.NewCSRFMiddleware(tokens),
		appmiddleware.NewRateLimitMiddleware(rateLimits),
		"http://localhost:3000",
		AdminComponents{
			Controller:      &adminControllerStub{},
			VideoController: videoController,
			Middleware:      appmiddleware.NewAdminMiddleware(),
		},
		VideoComponents{
			Controller:      &videoControllerStub{},
			SavedController: &savedVideoControllerStub{},
		},
	)
}

func TestRouterRegistersAdminVideoRoutes(t *testing.T) {
	e := newAdminVideoTestRouter(
		&adminVideoControllerStub{},
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	want := map[string]bool{
		"GET /admin/videos":                     false,
		"GET /admin/videos/:video_id":           false,
		"PATCH /admin/videos/:video_id/hide":    false,
		"PATCH /admin/videos/:video_id/restore": false,
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

func TestAdminVideoRoutesRequireAuthentication(t *testing.T) {
	videoController := &adminVideoControllerStub{}
	e := newAdminVideoTestRouter(
		videoController,
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if videoController.listCalled {
		t.Fatal("controller was called without authentication")
	}
}

func TestAdminVideoRoutesRejectGeneralUser(t *testing.T) {
	videoController := &adminVideoControllerStub{}
	users := &userUsecaseStub{
		getMeFunc: func(context.Context, uint64) (*entity.User, error) {
			return &entity.User{
				ID:     10,
				Role:   entity.RoleUser,
				Status: entity.StatusActive,
			}, nil
		},
		validateTokenVersionFunc: func(*entity.User, uint64) error {
			return nil
		},
	}
	tokens := &tokenServiceStub{
		parseFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
			return usecase.AccessTokenClaims{
				UserID:       10,
				Role:         entity.RoleUser,
				TokenVersion: 0,
			}, nil
		},
	}
	e := newAdminVideoTestRouter(videoController, users, tokens)

	req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if videoController.listCalled {
		t.Fatal("controller was called for a general user")
	}
}

func TestAdminVideoRoutesAllowActiveAdmin(t *testing.T) {
	videoController := &adminVideoControllerStub{}
	users := &userUsecaseStub{
		getMeFunc: func(context.Context, uint64) (*entity.User, error) {
			return &entity.User{
				ID:     99,
				Role:   entity.RoleAdmin,
				Status: entity.StatusActive,
			}, nil
		},
		validateTokenVersionFunc: func(*entity.User, uint64) error {
			return nil
		},
	}
	tokens := &tokenServiceStub{
		parseFunc: func(string, time.Time) (usecase.AccessTokenClaims, error) {
			return usecase.AccessTokenClaims{
				UserID:       99,
				Role:         entity.RoleAdmin,
				TokenVersion: 0,
			}, nil
		},
	}
	e := newAdminVideoTestRouter(videoController, users, tokens)

	req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !videoController.listCalled {
		t.Fatal("controller was not called for active admin")
	}
}
