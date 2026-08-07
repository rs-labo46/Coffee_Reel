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

type videoUsecaseControllerMock struct {
	listReelsFunc func(context.Context, *entity.User, usecase.PublicVideoListInput) (usecase.PublicVideoListResult, error)
	getDetailFunc func(context.Context, *entity.User, uint64) (usecase.PublicVideoResult, error)
}

func (m *videoUsecaseControllerMock) StartUpload(context.Context, *entity.User, usecase.StartUploadInput, string) (usecase.StartUploadResult, error) {
	panic("unexpected VideoUsecase.StartUpload call")
}
func (m *videoUsecaseControllerMock) CompleteUpload(context.Context, *entity.User, uint64) (usecase.VideoStateResult, error) {
	panic("unexpected VideoUsecase.CompleteUpload call")
}
func (m *videoUsecaseControllerMock) ListReels(ctx context.Context, viewer *entity.User, input usecase.PublicVideoListInput) (usecase.PublicVideoListResult, error) {
	if m.listReelsFunc == nil {
		panic("unexpected VideoUsecase.ListReels call")
	}
	return m.listReelsFunc(ctx, viewer, input)
}
func (m *videoUsecaseControllerMock) GetDetail(ctx context.Context, viewer *entity.User, videoID uint64) (usecase.PublicVideoResult, error) {
	if m.getDetailFunc == nil {
		panic("unexpected VideoUsecase.GetDetail call")
	}
	return m.getDetailFunc(ctx, viewer, videoID)
}
func (m *videoUsecaseControllerMock) ListMine(context.Context, *entity.User, usecase.VideoListInput) (usecase.OwnedVideoListResult, error) {
	panic("unexpected VideoUsecase.ListMine call")
}
func (m *videoUsecaseControllerMock) GetMine(context.Context, *entity.User, uint64) (usecase.OwnedVideoDetailResult, error) {
	panic("unexpected VideoUsecase.GetMine call")
}
func (m *videoUsecaseControllerMock) SetPrivate(context.Context, *entity.User, uint64) (usecase.VideoStateResult, error) {
	panic("unexpected VideoUsecase.SetPrivate call")
}
func (m *videoUsecaseControllerMock) Republish(context.Context, *entity.User, uint64) (usecase.VideoStateResult, error) {
	panic("unexpected VideoUsecase.Republish call")
}
func (m *videoUsecaseControllerMock) Delete(context.Context, *entity.User, uint64) error {
	panic("unexpected VideoUsecase.Delete call")
}

type videoLikeUsecaseControllerMock struct {
	likeFunc   func(context.Context, *entity.User, uint64) (usecase.VideoLikeResult, error)
	unlikeFunc func(context.Context, *entity.User, uint64) (usecase.VideoLikeResult, error)
}

func (m *videoLikeUsecaseControllerMock) Like(ctx context.Context, actor *entity.User, videoID uint64) (usecase.VideoLikeResult, error) {
	if m.likeFunc == nil {
		panic("unexpected VideoLikeUsecase.Like call")
	}
	return m.likeFunc(ctx, actor, videoID)
}
func (m *videoLikeUsecaseControllerMock) Unlike(ctx context.Context, actor *entity.User, videoID uint64) (usecase.VideoLikeResult, error) {
	if m.unlikeFunc == nil {
		panic("unexpected VideoLikeUsecase.Unlike call")
	}
	return m.unlikeFunc(ctx, actor, videoID)
}

func newSearchLikeVideoValidator(t *testing.T) validator.IVideoValidator {
	t.Helper()
	v, err := validator.NewVideoValidator(validator.VideoValidatorConfig{IdempotencyKeyMaxBytes: 128})
	if err != nil {
		t.Fatalf("NewVideoValidator() error = %v", err)
	}
	return v
}

func activeControllerVideoUser(id uint64) *entity.User {
	return &entity.User{ID: id, Role: entity.RoleUser, Status: entity.StatusActive}
}

func TestVideoControllerListReelsGuestSearchReturnsResultTypeAndLikeState(t *testing.T) {
	createdAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	videos := &videoUsecaseControllerMock{listReelsFunc: func(_ context.Context, viewer *entity.User, input usecase.PublicVideoListInput) (usecase.PublicVideoListResult, error) {
		if viewer != nil {
			t.Fatalf("viewer = %#v, want guest", viewer)
		}
		if input.Title != "latte" || input.Category == nil || *input.Category != entity.CategoryBrewing || input.Limit != 10 {
			t.Fatalf("input = %#v", input)
		}
		return usecase.PublicVideoListResult{
			Items: []usecase.PublicVideoResult{{
				ID: 4, UserID: 2, AuthorName: "author", Category: entity.CategoryBrewing,
				Title: "Latte", Description: "desc", CreatedAt: createdAt,
				Video:     usecase.ReadInfo{URL: "https://read.example/video"},
				Thumbnail: usecase.ReadInfo{URL: "https://read.example/thumb"},
				LikeCount: 5, IsLiked: false, IsSaved: false,
			}},
			ResultType: usecase.PublicSearchMatched,
		}, nil
	}}
	controller := NewVideoController(videos, newSearchLikeVideoValidator(t))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/videos?title=%20latte%20&category=brewing&limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := controller.ListReels(c); err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body publicVideoListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ResultType != usecase.PublicSearchMatched || len(body.Items) != 1 {
		t.Fatalf("body = %+v", body)
	}
	item := body.Items[0]
	if item.LikeCount != 5 || item.IsLiked || item.IsSaved || item.PlaybackURL == "" || item.ThumbnailURL == "" {
		t.Fatalf("item = %+v", item)
	}
}

func TestVideoControllerDetailReturnsLikeAndSavedState(t *testing.T) {
	createdAt := time.Date(2026, 8, 8, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	videos := &videoUsecaseControllerMock{getDetailFunc: func(_ context.Context, viewer *entity.User, videoID uint64) (usecase.PublicVideoResult, error) {
		if viewer != nil {
			t.Fatalf("viewer = %#v, want guest", viewer)
		}
		if videoID != 12 {
			t.Fatalf("videoID = %d, want 12", videoID)
		}
		return usecase.PublicVideoResult{
			ID:          12,
			UserID:      3,
			AuthorName:  "author",
			Category:    entity.CategoryLatteArt,
			Title:       "latte art",
			Description: "desc",
			CreatedAt:   createdAt,
			Video:       usecase.ReadInfo{URL: "https://read.example/video"},
			Thumbnail:   usecase.ReadInfo{URL: "https://read.example/thumb"},
			LikeCount:   8,
			IsLiked:     false,
			IsSaved:     false,
		}, nil
	}}
	controller := NewVideoController(videos, newSearchLikeVideoValidator(t))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/videos/12", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/videos/:video_id")
	c.SetParamNames("video_id")
	c.SetParamValues("12")

	if err := controller.Detail(c); err != nil {
		t.Fatalf("Detail() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body publicVideoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != 12 || body.LikeCount != 8 || body.IsLiked || body.IsSaved || body.PlaybackURL == "" || body.ThumbnailURL == "" {
		t.Fatalf("body = %+v", body)
	}
}

func TestVideoControllerListReelsPassesLoginViewer(t *testing.T) {
	viewer := activeControllerVideoUser(9)
	videos := &videoUsecaseControllerMock{listReelsFunc: func(_ context.Context, gotViewer *entity.User, input usecase.PublicVideoListInput) (usecase.PublicVideoListResult, error) {
		if gotViewer == nil || gotViewer.ID != viewer.ID {
			t.Fatalf("viewer = %#v", gotViewer)
		}
		if input.Title != "" || input.Category != nil || input.Limit != 20 {
			t.Fatalf("input = %#v", input)
		}
		return usecase.PublicVideoListResult{ResultType: usecase.PublicSearchAll}, nil
	}}
	controller := NewVideoController(videos, newSearchLikeVideoValidator(t))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/videos", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(userContextKey, viewer)

	if err := controller.ListReels(c); err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestVideoControllerListReelsRejectsExplicitEmptyTitle(t *testing.T) {
	called := false
	videos := &videoUsecaseControllerMock{listReelsFunc: func(context.Context, *entity.User, usecase.PublicVideoListInput) (usecase.PublicVideoListResult, error) {
		called = true
		return usecase.PublicVideoListResult{}, nil
	}}
	controller := NewVideoController(videos, newSearchLikeVideoValidator(t))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/videos?title=", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := controller.ListReels(c); err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("usecase was called for invalid title")
	}
}

func TestVideoLikeControllerReturnsUpdatedState(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		isLiked   bool
		likeCount int64
	}{
		{name: "like", method: http.MethodPut, isLiked: true, likeCount: 4},
		{name: "like retry", method: http.MethodPut, isLiked: true, likeCount: 4},
		{name: "unlike", method: http.MethodDelete, isLiked: false, likeCount: 3},
		{name: "unlike retry", method: http.MethodDelete, isLiked: false, likeCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			likes := &videoLikeUsecaseControllerMock{}
			change := func(_ context.Context, actor *entity.User, videoID uint64) (usecase.VideoLikeResult, error) {
				if actor == nil || actor.ID != 7 || videoID != 12 {
					t.Fatalf("actor=%#v videoID=%d", actor, videoID)
				}
				return usecase.VideoLikeResult{VideoID: 12, LikeCount: tt.likeCount, IsLiked: tt.isLiked}, nil
			}
			if tt.method == http.MethodPut {
				likes.likeFunc = change
			} else {
				likes.unlikeFunc = change
			}
			controller := NewVideoLikeController(likes, newSearchLikeVideoValidator(t))
			e := echo.New()
			req := httptest.NewRequest(tt.method, "/videos/12/like", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/videos/:video_id/like")
			c.SetParamNames("video_id")
			c.SetParamValues("12")
			c.Set(userContextKey, activeControllerVideoUser(7))

			var err error
			if tt.method == http.MethodPut {
				err = controller.Like(c)
			} else {
				err = controller.Unlike(c)
			}
			if err != nil {
				t.Fatalf("controller error = %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			var body videoLikeResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.VideoID != 12 || body.LikeCount != tt.likeCount || body.IsLiked != tt.isLiked {
				t.Fatalf("body = %+v", body)
			}
		})
	}
}

func TestVideoLikeControllerMapsErrorsWithoutLeakingInternalDetails(t *testing.T) {
	tests := []struct {
		name       string
		withUser   bool
		videoID    string
		usecaseErr error
		wantStatus int
	}{
		{name: "unauthenticated", withUser: false, videoID: "12", wantStatus: http.StatusUnauthorized},
		{name: "invalid video id", withUser: true, videoID: "0", wantStatus: http.StatusBadRequest},
		{name: "suspended", withUser: true, videoID: "12", usecaseErr: entity.ErrUserSuspended, wantStatus: http.StatusUnauthorized},
		{name: "non public video", withUser: true, videoID: "12", usecaseErr: entity.ErrVideoNotFound, wantStatus: http.StatusNotFound},
		{name: "internal db error", withUser: true, videoID: "12", usecaseErr: errors.New("pq: constraint uq_video_likes_user_video failed"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			likes := &videoLikeUsecaseControllerMock{likeFunc: func(context.Context, *entity.User, uint64) (usecase.VideoLikeResult, error) {
				return usecase.VideoLikeResult{}, tt.usecaseErr
			}}
			controller := NewVideoLikeController(likes, newSearchLikeVideoValidator(t))
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/videos/"+tt.videoID+"/like", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/videos/:video_id/like")
			c.SetParamNames("video_id")
			c.SetParamValues(tt.videoID)
			if tt.withUser {
				c.Set(userContextKey, activeControllerVideoUser(7))
			}

			if err := controller.Like(c); err != nil {
				t.Fatalf("Like() error = %v", err)
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			body := rec.Body.String()
			for _, secret := range []string{"pq:", "uq_video_likes_user_video", "constraint"} {
				if strings.Contains(body, secret) {
					t.Fatalf("response leaked internal detail %q: %s", secret, body)
				}
			}
		})
	}
}
