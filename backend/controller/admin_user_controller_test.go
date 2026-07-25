package controller

import (
	"coffee-reel/entity"
	"coffee-reel/usecase"
	"coffee-reel/validator"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type adminUserUsecaseStub struct {
	createAdminFunc func(context.Context, string, string, string) (*entity.User, bool, error)
	listUsersFunc   func(context.Context, *entity.User, usecase.AdminUserListInput) (usecase.AdminUserListResult, error)
	getDetailFunc   func(context.Context, *entity.User, uint64) (usecase.AdminUserDetailResult, error)
	suspendFunc     func(context.Context, *entity.User, uint64, string, string) (usecase.AdminUserStatusResult, error)
	resumeFunc      func(context.Context, *entity.User, uint64, string, string) (usecase.AdminUserStatusResult, error)
}

func (s *adminUserUsecaseStub) CreateAdmin(ctx context.Context, name, email, password string) (*entity.User, bool, error) {
	if s.createAdminFunc == nil {
		panic("unexpected CreateAdmin call")
	}
	return s.createAdminFunc(ctx, name, email, password)
}

func (s *adminUserUsecaseStub) ListUsers(ctx context.Context, actor *entity.User, input usecase.AdminUserListInput) (usecase.AdminUserListResult, error) {
	if s.listUsersFunc == nil {
		panic("unexpected ListUsers call")
	}
	return s.listUsersFunc(ctx, actor, input)
}

func (s *adminUserUsecaseStub) GetUserDetail(ctx context.Context, actor *entity.User, targetUserID uint64) (usecase.AdminUserDetailResult, error) {
	if s.getDetailFunc == nil {
		panic("unexpected GetUserDetail call")
	}
	return s.getDetailFunc(ctx, actor, targetUserID)
}

func (s *adminUserUsecaseStub) SuspendUser(ctx context.Context, actor *entity.User, targetUserID uint64, reason, requestID string) (usecase.AdminUserStatusResult, error) {
	if s.suspendFunc == nil {
		panic("unexpected SuspendUser call")
	}
	return s.suspendFunc(ctx, actor, targetUserID, reason, requestID)
}

func (s *adminUserUsecaseStub) ResumeUser(ctx context.Context, actor *entity.User, targetUserID uint64, reason, requestID string) (usecase.AdminUserStatusResult, error) {
	if s.resumeFunc == nil {
		panic("unexpected ResumeUser call")
	}
	return s.resumeFunc(ctx, actor, targetUserID, reason, requestID)
}

func newAdminUserControllerForTest(users usecase.IAdminUserUsecase) IAdminUserController {
	userValidator := validator.NewUserValidator()
	adminValidator := validator.NewAdminUserValidator(userValidator)
	return NewAdminUserController(users, adminValidator)
}

func newAdminEchoContext(method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderXRequestID, "request-controller-admin")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(userContextKey, &entity.User{ID: 99, Role: entity.RoleAdmin, Status: entity.StatusActive})
	return c, rec
}

func TestAdminUserControllerListUsers(t *testing.T) {
	nextCursor := "next-cursor"
	users := &adminUserUsecaseStub{listUsersFunc: func(_ context.Context, actor *entity.User, input usecase.AdminUserListInput) (usecase.AdminUserListResult, error) {
		if actor.ID != 99 || input.Limit != 20 || input.Cursor != nil {
			t.Fatalf("actor=%#v input=%#v", actor, input)
		}
		return usecase.AdminUserListResult{
			Items: []usecase.AdminUserListItem{{
				ID: 1, Name: "Alice", Email: "alice@example.com", Status: entity.StatusActive,
				CreatedAt: time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC),
			}},
			NextCursor: &nextCursor,
			HasMore:    true,
		}, nil
	}}
	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users", "")

	if err := controller.ListUsers(c); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminUserListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.NextCursor == nil || !response.HasMore {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminUserControllerListUsersRejectsInvalidQuery(t *testing.T) {
	controller := newAdminUserControllerForTest(&adminUserUsecaseStub{})
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users?limit=0", "")

	if err := controller.ListUsers(c); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserControllerGetUserDetail(t *testing.T) {
	createdAt := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	users := &adminUserUsecaseStub{getDetailFunc: func(_ context.Context, actor *entity.User, targetUserID uint64) (usecase.AdminUserDetailResult, error) {
		if actor.ID != 99 || targetUserID != 10 {
			t.Fatalf("actor=%#v targetUserID=%d", actor, targetUserID)
		}
		return usecase.AdminUserDetailResult{
			ID:        10,
			Name:      "Alice",
			Email:     "alice@example.com",
			Status:    entity.StatusActive,
			CreatedAt: createdAt,
			Videos:    []usecase.AdminUserVideoItem{},
		}, nil
	}}
	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users/10", "")
	c.SetPath("/admin/users/:user_id")
	c.SetParamNames("user_id")
	c.SetParamValues("10")

	if err := controller.GetUserDetail(c); err != nil {
		t.Fatalf("GetUserDetail() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminUserDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 10 || response.Name != "Alice" || response.Status != entity.StatusActive {
		t.Fatalf("response = %#v", response)
	}
	if response.Videos == nil || len(response.Videos) != 0 {
		t.Fatalf("videos = %#v, want empty array", response.Videos)
	}
	if strings.Contains(rec.Body.String(), "updated_at") || strings.Contains(rec.Body.String(), "token_version") {
		t.Fatalf("detail response exposed an unspecified or secret field: %s", rec.Body.String())
	}
}

func TestAdminUserControllerGetUserDetailReturnsNotFound(t *testing.T) {
	users := &adminUserUsecaseStub{getDetailFunc: func(context.Context, *entity.User, uint64) (usecase.AdminUserDetailResult, error) {
		return usecase.AdminUserDetailResult{}, entity.ErrUserNotFound
	}}
	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users/10", "")
	c.SetPath("/admin/users/:user_id")
	c.SetParamNames("user_id")
	c.SetParamValues("10")

	if err := controller.GetUserDetail(c); err != nil {
		t.Fatalf("GetUserDetail() error = %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserControllerSuspendUser(t *testing.T) {
	updatedAt := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	users := &adminUserUsecaseStub{suspendFunc: func(_ context.Context, actor *entity.User, targetUserID uint64, reason, requestID string) (usecase.AdminUserStatusResult, error) {
		if actor.ID != 99 || targetUserID != 10 || reason != "規約違反" || requestID != "request-controller-admin" {
			t.Fatalf("inputs = %#v %d %q %q", actor, targetUserID, reason, requestID)
		}
		return usecase.AdminUserStatusResult{ID: 10, Status: entity.StatusSuspended, UpdatedAt: updatedAt}, nil
	}}
	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodPatch, "/admin/users/10/suspend", `{"reason":"  規約違反  "}`)
	c.SetPath("/admin/users/:user_id/suspend")
	c.SetParamNames("user_id")
	c.SetParamValues("10")

	if err := controller.SuspendUser(c); err != nil {
		t.Fatalf("SuspendUser() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminUserStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 10 || response.Status != entity.StatusSuspended || !response.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminUserControllerMapsStatusConflict(t *testing.T) {
	users := &adminUserUsecaseStub{resumeFunc: func(context.Context, *entity.User, uint64, string, string) (usecase.AdminUserStatusResult, error) {
		return usecase.AdminUserStatusResult{}, entity.ErrUserStatusConflict
	}}
	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodPatch, "/admin/users/10/resume", `{"reason":"確認完了"}`)
	c.SetPath("/admin/users/:user_id/resume")
	c.SetParamNames("user_id")
	c.SetParamValues("10")

	if err := controller.ResumeUser(c); err != nil {
		t.Fatalf("ResumeUser() error = %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "user_status_conflict" || response.RequestID != "request-controller-admin" {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminUserControllerDoesNotExposeInternalError(t *testing.T) {
	users := &adminUserUsecaseStub{listUsersFunc: func(context.Context, *entity.User, usecase.AdminUserListInput) (usecase.AdminUserListResult, error) {
		return usecase.AdminUserListResult{}, errors.New("sql: secret constraint uq_password")
	}}
	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users", "")

	if err := controller.ListUsers(c); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "constraint") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserControllerResumeUser(t *testing.T) {
	updatedAt := time.Date(
		2026,
		7,
		23,
		21,
		0,
		0,
		456,
		time.FixedZone("JST", 9*60*60),
	)

	users := &adminUserUsecaseStub{
		resumeFunc: func(
			_ context.Context,
			actor *entity.User,
			targetUserID uint64,
			reason string,
			requestID string,
		) (usecase.AdminUserStatusResult, error) {
			if actor.ID != 99 ||
				targetUserID != 10 ||
				reason != "確認完了" ||
				requestID != "request-controller-admin" {
				t.Fatalf(
					"inputs = %#v %d %q %q",
					actor,
					targetUserID,
					reason,
					requestID,
				)
			}

			return usecase.AdminUserStatusResult{
				ID:        10,
				Status:    entity.StatusActive,
				UpdatedAt: updatedAt,
			}, nil
		},
	}

	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(
		http.MethodPatch,
		"/admin/users/10/resume",
		`{"reason":"  確認完了  "}`,
	)
	c.SetPath("/admin/users/:user_id/resume")
	c.SetParamNames("user_id")
	c.SetParamValues("10")

	if err := controller.ResumeUser(c); err != nil {
		t.Fatalf("ResumeUser() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminUserStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if response.ID != 10 || response.Status != entity.StatusActive {
		t.Fatalf("response = %#v", response)
	}
	if !response.UpdatedAt.Equal(updatedAt.UTC()) ||
		response.UpdatedAt.Location() != time.UTC {
		t.Fatalf(
			"UpdatedAt = %s, want UTC %s",
			response.UpdatedAt,
			updatedAt.UTC(),
		)
	}
}

func TestAdminUserControllerRejectsInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		userID string
		call   func(IAdminUserController, echo.Context) error
	}{
		{
			name:   "get user detail",
			method: http.MethodGet,
			target: "/admin/users/abc",
			userID: "abc",
			call: func(controller IAdminUserController, c echo.Context) error {
				return controller.GetUserDetail(c)
			},
		},
		{
			name:   "suspend user",
			method: http.MethodPatch,
			target: "/admin/users/0/suspend",
			userID: "0",
			call: func(controller IAdminUserController, c echo.Context) error {
				return controller.SuspendUser(c)
			},
		},
		{
			name:   "resume user",
			method: http.MethodPatch,
			target: "/admin/users/-1/resume",
			userID: "-1",
			call: func(controller IAdminUserController, c echo.Context) error {
				return controller.ResumeUser(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := newAdminUserControllerForTest(
				&adminUserUsecaseStub{},
			)
			c, rec := newAdminEchoContext(tt.method, tt.target, "")
			c.SetParamNames("user_id")
			c.SetParamValues(tt.userID)

			if err := tt.call(controller, c); err != nil {
				t.Fatalf("controller error = %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf(
					"status = %d body=%s",
					rec.Code,
					rec.Body.String(),
				)
			}

			var response apiErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != "validation_failed" ||
				response.RequestID != "request-controller-admin" {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}

func TestAdminUserControllerRejectsInvalidStatusRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed JSON",
			body: `{"reason":`,
		},
		{
			name: "blank reason",
			body: `{"reason":"   "}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := newAdminUserControllerForTest(
				&adminUserUsecaseStub{},
			)
			c, rec := newAdminEchoContext(
				http.MethodPatch,
				"/admin/users/10/suspend",
				tt.body,
			)
			c.SetPath("/admin/users/:user_id/suspend")
			c.SetParamNames("user_id")
			c.SetParamValues("10")

			if err := controller.SuspendUser(c); err != nil {
				t.Fatalf("SuspendUser() error = %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf(
					"status = %d body=%s",
					rec.Code,
					rec.Body.String(),
				)
			}

			var response apiErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != "validation_failed" ||
				response.RequestID != "request-controller-admin" {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}

func TestAdminUserControllerRejectsMissingAuthenticatedUser(t *testing.T) {
	controller := newAdminUserControllerForTest(&adminUserUsecaseStub{})
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users", "")
	c.Set(userContextKey, nil)

	if err := controller.ListUsers(c); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "unauthorized" ||
		response.RequestID != "request-controller-admin" {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminUserControllerMapsForbiddenErrors(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		target   string
		body     string
		userID   string
		wantCode string
		users    *adminUserUsecaseStub
		call     func(IAdminUserController, echo.Context) error
	}{
		{
			name:     "admin required",
			method:   http.MethodGet,
			target:   "/admin/users",
			wantCode: "admin_required",
			users: &adminUserUsecaseStub{
				listUsersFunc: func(
					context.Context,
					*entity.User,
					usecase.AdminUserListInput,
				) (usecase.AdminUserListResult, error) {
					return usecase.AdminUserListResult{},
						entity.ErrAdminRequired
				},
			},
			call: func(controller IAdminUserController, c echo.Context) error {
				return controller.ListUsers(c)
			},
		},
		{
			name:     "user management forbidden",
			method:   http.MethodPatch,
			target:   "/admin/users/10/suspend",
			body:     `{"reason":"規約違反"}`,
			userID:   "10",
			wantCode: "user_management_forbidden",
			users: &adminUserUsecaseStub{
				suspendFunc: func(
					context.Context,
					*entity.User,
					uint64,
					string,
					string,
				) (usecase.AdminUserStatusResult, error) {
					return usecase.AdminUserStatusResult{},
						entity.ErrUserManagementForbidden
				},
			},
			call: func(controller IAdminUserController, c echo.Context) error {
				return controller.SuspendUser(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := newAdminUserControllerForTest(tt.users)
			c, rec := newAdminEchoContext(
				tt.method,
				tt.target,
				tt.body,
			)

			if tt.userID != "" {
				c.SetParamNames("user_id")
				c.SetParamValues(tt.userID)
			}

			if err := tt.call(controller, c); err != nil {
				t.Fatalf("controller error = %v", err)
			}
			if rec.Code != http.StatusForbidden {
				t.Fatalf(
					"status = %d body=%s",
					rec.Code,
					rec.Body.String(),
				)
			}

			var response apiErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tt.wantCode ||
				response.RequestID != "request-controller-admin" {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}

func TestAdminUserControllerGetUserDetailMapsVideosAndUTC(t *testing.T) {
	userCreatedAt := time.Date(
		2026,
		7,
		23,
		19,
		0,
		0,
		123,
		time.FixedZone("JST", 9*60*60),
	)
	videoCreatedAt := time.Date(
		2026,
		7,
		24,
		20,
		30,
		0,
		456,
		time.FixedZone("JST", 9*60*60),
	)

	users := &adminUserUsecaseStub{
		getDetailFunc: func(
			_ context.Context,
			actor *entity.User,
			targetUserID uint64,
		) (usecase.AdminUserDetailResult, error) {
			if actor.ID != 99 || targetUserID != 10 {
				t.Fatalf(
					"actor=%#v targetUserID=%d",
					actor,
					targetUserID,
				)
			}

			return usecase.AdminUserDetailResult{
				ID:        10,
				Name:      "Alice",
				Email:     "alice@example.com",
				Status:    entity.StatusActive,
				CreatedAt: userCreatedAt,
				Videos: []usecase.AdminUserVideoItem{
					{
						ID:               30,
						Title:            "Coffee Video",
						ProcessingStatus: "ready",
						PublishStatus:    "published",
						CreatedAt:        videoCreatedAt,
					},
				},
			}, nil
		},
	}

	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(
		http.MethodGet,
		"/admin/users/10",
		"",
	)
	c.SetPath("/admin/users/:user_id")
	c.SetParamNames("user_id")
	c.SetParamValues("10")

	if err := controller.GetUserDetail(c); err != nil {
		t.Fatalf("GetUserDetail() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminUserDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if !response.CreatedAt.Equal(userCreatedAt.UTC()) ||
		response.CreatedAt.Location() != time.UTC {
		t.Fatalf(
			"CreatedAt = %s, want UTC %s",
			response.CreatedAt,
			userCreatedAt.UTC(),
		)
	}
	if len(response.Videos) != 1 {
		t.Fatalf("Videos length = %d, want 1", len(response.Videos))
	}

	video := response.Videos[0]
	if video.ID != 30 ||
		video.Title != "Coffee Video" ||
		video.ProcessingStatus != "ready" ||
		video.PublishStatus != "published" {
		t.Fatalf("video = %#v", video)
	}
	if !video.CreatedAt.Equal(videoCreatedAt.UTC()) ||
		video.CreatedAt.Location() != time.UTC {
		t.Fatalf(
			"video CreatedAt = %s, want UTC %s",
			video.CreatedAt,
			videoCreatedAt.UTC(),
		)
	}
}

func TestAdminUserControllerListUsersReturnsEmptyArray(t *testing.T) {
	users := &adminUserUsecaseStub{
		listUsersFunc: func(
			context.Context,
			*entity.User,
			usecase.AdminUserListInput,
		) (usecase.AdminUserListResult, error) {
			return usecase.AdminUserListResult{
				Items:   nil,
				HasMore: false,
			}, nil
		},
	}

	controller := newAdminUserControllerForTest(users)
	c, rec := newAdminEchoContext(http.MethodGet, "/admin/users", "")

	if err := controller.ListUsers(c); err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminUserListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Items == nil || len(response.Items) != 0 {
		t.Fatalf("Items = %#v, want empty array", response.Items)
	}
	if response.NextCursor != nil || response.HasMore {
		t.Fatalf("response = %#v", response)
	}
}
