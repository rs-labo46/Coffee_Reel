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
	"coffee-reel/validator"

	"github.com/labstack/echo/v4"
)

type adminVideoUsecaseStub struct {
	listFunc    func(context.Context, *entity.User, usecase.AdminVideoListInput) (usecase.AdminVideoListResult, error)
	getFunc     func(context.Context, *entity.User, uint64) (usecase.AdminVideoDetailResult, error)
	hideFunc    func(context.Context, *entity.User, uint64, string, string) (usecase.AdminVideoStateResult, error)
	restoreFunc func(context.Context, *entity.User, uint64, string, string) (usecase.AdminVideoStateResult, error)
}

func (s *adminVideoUsecaseStub) List(
	ctx context.Context,
	actor *entity.User,
	input usecase.AdminVideoListInput,
) (usecase.AdminVideoListResult, error) {
	if s.listFunc == nil {
		panic("unexpected AdminVideoUsecase.List call")
	}
	return s.listFunc(ctx, actor, input)
}

func (s *adminVideoUsecaseStub) Get(
	ctx context.Context,
	actor *entity.User,
	videoID uint64,
) (usecase.AdminVideoDetailResult, error) {
	if s.getFunc == nil {
		panic("unexpected AdminVideoUsecase.Get call")
	}
	return s.getFunc(ctx, actor, videoID)
}

func (s *adminVideoUsecaseStub) Hide(
	ctx context.Context,
	actor *entity.User,
	videoID uint64,
	reason string,
	requestID string,
) (usecase.AdminVideoStateResult, error) {
	if s.hideFunc == nil {
		panic("unexpected AdminVideoUsecase.Hide call")
	}
	return s.hideFunc(ctx, actor, videoID, reason, requestID)
}

func (s *adminVideoUsecaseStub) Restore(
	ctx context.Context,
	actor *entity.User,
	videoID uint64,
	reason string,
	requestID string,
) (usecase.AdminVideoStateResult, error) {
	if s.restoreFunc == nil {
		panic("unexpected AdminVideoUsecase.Restore call")
	}
	return s.restoreFunc(ctx, actor, videoID, reason, requestID)
}

func newAdminVideoControllerForTest(
	videos usecase.IAdminVideoUsecase,
) IAdminVideoController {
	return NewAdminVideoController(
		videos,
		validator.NewAdminVideoValidator(),
	)
}

func newAdminVideoEchoContext(
	method string,
	target string,
	body string,
	requestIDValue string,
) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if requestIDValue != "" {
		req.Header.Set(echo.HeaderXRequestID, requestIDValue)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(userContextKey, &entity.User{
		ID:     99,
		Role:   entity.RoleAdmin,
		Status: entity.StatusActive,
	})
	return c, rec
}

func setAdminVideoPathParam(
	c echo.Context,
	path string,
	videoID string,
) {
	c.SetPath(path)
	c.SetParamNames("video_id")
	c.SetParamValues(videoID)
}

func TestAdminVideoControllerList(t *testing.T) {
	nextCursor := "next-cursor"
	createdAt := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)

	videos := &adminVideoUsecaseStub{
		listFunc: func(
			_ context.Context,
			actor *entity.User,
			input usecase.AdminVideoListInput,
		) (usecase.AdminVideoListResult, error) {
			if actor.ID != 99 || input.Limit != 20 || input.Cursor != nil {
				t.Fatalf("actor=%#v input=%#v", actor, input)
			}
			return usecase.AdminVideoListResult{
				Items: []usecase.AdminVideoListItem{
					{
						ID: 10,
						Author: usecase.AdminVideoAuthorResult{
							ID:     2,
							Name:   "Alice",
							Status: entity.StatusActive,
						},
						Title:            "title",
						Description:      "description",
						Category:         entity.CategoryBrewing,
						ProcessingStatus: entity.VideoProcessingReady,
						PublishStatus:    entity.VideoPublishPublished,
						CreatedAt:        createdAt,
						UpdatedAt:        createdAt,
					},
				},
				NextCursor: &nextCursor,
				HasMore:    true,
			}, nil
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodGet,
		"/admin/videos",
		"",
		"request-admin-video-list",
	)

	if err := controller.List(c); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminVideoListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 ||
		response.Items[0].ID != 10 ||
		response.Items[0].Author.Name != "Alice" ||
		response.NextCursor == nil ||
		!response.HasMore {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(rec.Body.String(), "object_key") ||
		strings.Contains(rec.Body.String(), "playback_url") {
		t.Fatalf("list exposed internal or detail-only fields: %s", rec.Body.String())
	}
}

func TestAdminVideoControllerListReturnsEmptyArray(t *testing.T) {
	videos := &adminVideoUsecaseStub{
		listFunc: func(
			context.Context,
			*entity.User,
			usecase.AdminVideoListInput,
		) (usecase.AdminVideoListResult, error) {
			return usecase.AdminVideoListResult{
				Items: []usecase.AdminVideoListItem{},
			}, nil
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodGet,
		"/admin/videos",
		"",
		"request-admin-video-list-empty",
	)

	if err := controller.List(c); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var response adminVideoListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Items == nil || len(response.Items) != 0 {
		t.Fatalf("items = %#v, want empty array", response.Items)
	}
	if response.NextCursor != nil || response.HasMore {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminVideoControllerDetail(t *testing.T) {
	playbackURL := "https://storage.example/video"
	thumbnailURL := "https://storage.example/thumbnail"
	createdAt := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)

	videos := &adminVideoUsecaseStub{
		getFunc: func(
			_ context.Context,
			actor *entity.User,
			videoID uint64,
		) (usecase.AdminVideoDetailResult, error) {
			if actor.ID != 99 || videoID != 10 {
				t.Fatalf("actor=%#v videoID=%d", actor, videoID)
			}
			return usecase.AdminVideoDetailResult{
				ID: 10,
				Author: usecase.AdminVideoAuthorResult{
					ID:     2,
					Name:   "Alice",
					Status: entity.StatusActive,
				},
				Title:            "title",
				Description:      "description",
				Category:         entity.CategoryBrewing,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishHidden,
				PlaybackURL:      &playbackURL,
				ThumbnailURL:     &thumbnailURL,
				CreatedAt:        createdAt,
				UpdatedAt:        createdAt,
			}, nil
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodGet,
		"/admin/videos/10",
		"",
		"request-admin-video-detail",
	)
	setAdminVideoPathParam(c, "/admin/videos/:video_id", "10")

	if err := controller.Detail(c); err != nil {
		t.Fatalf("Detail() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminVideoDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 10 ||
		response.PublishStatus != entity.VideoPublishHidden ||
		response.PlaybackURL == nil ||
		response.ThumbnailURL == nil {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(rec.Body.String(), "object_key") ||
		strings.Contains(rec.Body.String(), "original") {
		t.Fatalf("detail exposed Object Key: %s", rec.Body.String())
	}
}

func TestAdminVideoControllerHide(t *testing.T) {
	updatedAt := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	videos := &adminVideoUsecaseStub{
		hideFunc: func(
			_ context.Context,
			actor *entity.User,
			videoID uint64,
			reason string,
			requestID string,
		) (usecase.AdminVideoStateResult, error) {
			if actor.ID != 99 ||
				videoID != 10 ||
				reason != "規約違反" ||
				requestID != "request-admin-video-hide" {
				t.Fatalf(
					"actor=%#v videoID=%d reason=%q requestID=%q",
					actor,
					videoID,
					reason,
					requestID,
				)
			}
			return usecase.AdminVideoStateResult{
				ID:               10,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishHidden,
				UpdatedAt:        updatedAt,
			}, nil
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodPatch,
		"/admin/videos/10/hide",
		`{"reason":"  規約違反  "}`,
		"request-admin-video-hide",
	)
	setAdminVideoPathParam(c, "/admin/videos/:video_id/hide", "10")

	if err := controller.Hide(c); err != nil {
		t.Fatalf("Hide() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminVideoStateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 10 ||
		response.ProcessingStatus != entity.VideoProcessingReady ||
		response.PublishStatus != entity.VideoPublishHidden ||
		!response.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminVideoControllerRejectsUnknownReasonField(t *testing.T) {
	controller := newAdminVideoControllerForTest(&adminVideoUsecaseStub{})
	c, rec := newAdminVideoEchoContext(
		http.MethodPatch,
		"/admin/videos/10/hide",
		`{"reason":"規約違反","publish_status":"published"}`,
		"request-admin-video-unknown-field",
	)
	setAdminVideoPathParam(c, "/admin/videos/:video_id/hide", "10")

	if err := controller.Hide(c); err != nil {
		t.Fatalf("Hide() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminVideoControllerMapsOwnerSuspended(t *testing.T) {
	videos := &adminVideoUsecaseStub{
		restoreFunc: func(
			context.Context,
			*entity.User,
			uint64,
			string,
			string,
		) (usecase.AdminVideoStateResult, error) {
			return usecase.AdminVideoStateResult{}, entity.ErrUserSuspended
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodPatch,
		"/admin/videos/10/restore",
		`{"reason":"確認完了"}`,
		"request-admin-video-restore",
	)
	setAdminVideoPathParam(c, "/admin/videos/:video_id/restore", "10")

	if err := controller.Restore(c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response apiErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "video_owner_suspended" ||
		response.RequestID != "request-admin-video-restore" {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminVideoControllerRejectsMissingRequestID(t *testing.T) {
	controller := newAdminVideoControllerForTest(&adminVideoUsecaseStub{})
	c, rec := newAdminVideoEchoContext(
		http.MethodPatch,
		"/admin/videos/10/hide",
		`{"reason":"規約違反"}`,
		"",
	)
	setAdminVideoPathParam(c, "/admin/videos/:video_id/hide", "10")

	if err := controller.Hide(c); err != nil {
		t.Fatalf("Hide() error = %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminVideoControllerRejectsMissingActor(t *testing.T) {
	controller := newAdminVideoControllerForTest(&adminVideoUsecaseStub{})
	c, rec := newAdminVideoEchoContext(
		http.MethodGet,
		"/admin/videos",
		"",
		"request-admin-video-no-actor",
	)
	c.Set(userContextKey, nil)

	if err := controller.List(c); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminVideoControllerDoesNotExposeInternalError(t *testing.T) {
	videos := &adminVideoUsecaseStub{
		listFunc: func(
			context.Context,
			*entity.User,
			usecase.AdminVideoListInput,
		) (usecase.AdminVideoListResult, error) {
			return usecase.AdminVideoListResult{}, errors.New(
				"sql: constraint chk_secret object_key=videos/secret",
			)
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodGet,
		"/admin/videos",
		"",
		"request-admin-video-internal",
	)

	if err := controller.List(c); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, secret := range []string{"constraint", "chk_secret", "object_key", "videos/secret"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("response exposed %q: %s", secret, rec.Body.String())
		}
	}
}

func TestAdminVideoControllerRestore(t *testing.T) {
	updatedAt := time.Date(2026, 8, 6, 3, 0, 0, 0, time.UTC)
	videos := &adminVideoUsecaseStub{
		restoreFunc: func(
			_ context.Context,
			actor *entity.User,
			videoID uint64,
			reason string,
			requestID string,
		) (usecase.AdminVideoStateResult, error) {
			if actor.ID != 99 ||
				videoID != 10 ||
				reason != "再確認済み" ||
				requestID != "request-admin-video-restore-success" {
				t.Fatalf(
					"actor=%#v videoID=%d reason=%q requestID=%q",
					actor,
					videoID,
					reason,
					requestID,
				)
			}
			return usecase.AdminVideoStateResult{
				ID:               10,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishPublished,
				UpdatedAt:        updatedAt,
			}, nil
		},
	}

	controller := newAdminVideoControllerForTest(videos)
	c, rec := newAdminVideoEchoContext(
		http.MethodPatch,
		"/admin/videos/10/restore",
		`{"reason":"  再確認済み  "}`,
		"request-admin-video-restore-success",
	)
	setAdminVideoPathParam(c, "/admin/videos/:video_id/restore", "10")

	if err := controller.Restore(c); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var response adminVideoStateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 10 ||
		response.ProcessingStatus != entity.VideoProcessingReady ||
		response.PublishStatus != entity.VideoPublishPublished ||
		!response.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminVideoControllerMapsVideoDomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		domainErr  error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "not found",
			domainErr:  entity.ErrVideoNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "video_not_found",
		},
		{
			name:       "state conflict",
			domainErr:  entity.ErrVideoStateConflict,
			wantStatus: http.StatusConflict,
			wantCode:   "video_state_conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videos := &adminVideoUsecaseStub{
				getFunc: func(
					context.Context,
					*entity.User,
					uint64,
				) (usecase.AdminVideoDetailResult, error) {
					return usecase.AdminVideoDetailResult{}, tt.domainErr
				},
			}
			controller := newAdminVideoControllerForTest(videos)
			c, rec := newAdminVideoEchoContext(
				http.MethodGet,
				"/admin/videos/10",
				"",
				"request-admin-video-domain-error",
			)
			setAdminVideoPathParam(c, "/admin/videos/:video_id", "10")

			if err := controller.Detail(c); err != nil {
				t.Fatalf("Detail() error = %v", err)
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}

			var response apiErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tt.wantCode ||
				response.RequestID != "request-admin-video-domain-error" {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}
