package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type adminVideoRepositoryMock struct {
	listFunc     func(context.Context, int, *repository.AdminVideoCursor) (repository.AdminVideoListResult, error)
	findByIDFunc func(context.Context, uint64) (*repository.AdminVideoDetail, error)
	hideFunc     func(context.Context, uint64, uint64, string, string, time.Time) (*repository.AdminVideoState, error)
	restoreFunc  func(context.Context, uint64, uint64, string, string, time.Time) (*repository.AdminVideoState, error)
}

func (m *adminVideoRepositoryMock) List(
	ctx context.Context,
	limit int,
	cursor *repository.AdminVideoCursor,
) (repository.AdminVideoListResult, error) {
	if m.listFunc == nil {
		panic("unexpected AdminVideoRepository.List call")
	}
	return m.listFunc(ctx, limit, cursor)
}

func (m *adminVideoRepositoryMock) FindByID(
	ctx context.Context,
	videoID uint64,
) (*repository.AdminVideoDetail, error) {
	if m.findByIDFunc == nil {
		panic("unexpected AdminVideoRepository.FindByID call")
	}
	return m.findByIDFunc(ctx, videoID)
}

func (m *adminVideoRepositoryMock) Hide(
	ctx context.Context,
	adminUserID uint64,
	videoID uint64,
	reason string,
	requestID string,
	now time.Time,
) (*repository.AdminVideoState, error) {
	if m.hideFunc == nil {
		panic("unexpected AdminVideoRepository.Hide call")
	}
	return m.hideFunc(ctx, adminUserID, videoID, reason, requestID, now)
}

func (m *adminVideoRepositoryMock) Restore(
	ctx context.Context,
	adminUserID uint64,
	videoID uint64,
	reason string,
	requestID string,
	now time.Time,
) (*repository.AdminVideoState, error) {
	if m.restoreFunc == nil {
		panic("unexpected AdminVideoRepository.Restore call")
	}
	return m.restoreFunc(ctx, adminUserID, videoID, reason, requestID, now)
}

func activeAdminVideoActor(id uint64) *entity.User {
	return &entity.User{
		ID:     id,
		Role:   entity.RoleAdmin,
		Status: entity.StatusActive,
	}
}

func newAdminVideoUsecaseForTest(
	t *testing.T,
	videos repository.IAdminVideoRepository,
	storage repository.IObjectStorageRepository,
) IAdminVideoUsecase {
	t.Helper()

	result, err := NewAdminVideoUsecase(
		videos,
		storage,
		AdminVideoUsecaseConfig{ReadURLTTL: 10 * time.Minute},
	)
	if err != nil {
		t.Fatalf("NewAdminVideoUsecase() error = %v", err)
	}
	return result
}

func TestNewAdminVideoUsecaseRejectsInvalidConfiguration(t *testing.T) {
	validVideos := &adminVideoRepositoryMock{}
	validStorage := &objectStorageRepositoryMock{}

	tests := []struct {
		name    string
		videos  repository.IAdminVideoRepository
		storage repository.IObjectStorageRepository
		ttl     time.Duration
	}{
		{name: "nil video repository", storage: validStorage, ttl: time.Minute},
		{name: "nil storage repository", videos: validVideos, ttl: time.Minute},
		{name: "zero read URL TTL", videos: validVideos, storage: validStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewAdminVideoUsecase(
				tt.videos,
				tt.storage,
				AdminVideoUsecaseConfig{ReadURLTTL: tt.ttl},
			)
			if err == nil {
				t.Fatal("error = nil, want configuration error")
			}
			if result != nil {
				t.Fatalf("result = %#v, want nil", result)
			}
		})
	}
}

func TestAdminVideoUsecaseListBuildsCursor(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	inputCursorTime := time.Date(2026, 8, 6, 10, 0, 0, 0, jst)
	firstCreatedAt := time.Date(2026, 8, 6, 0, 30, 0, 0, time.UTC)
	lastCreatedAt := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)

	videos := &adminVideoRepositoryMock{
		listFunc: func(
			_ context.Context,
			limit int,
			cursor *repository.AdminVideoCursor,
		) (repository.AdminVideoListResult, error) {
			if limit != 2 {
				t.Fatalf("limit = %d, want 2", limit)
			}
			if cursor == nil ||
				!cursor.CreatedAt.Equal(inputCursorTime.UTC()) ||
				cursor.CreatedAt.Location() != time.UTC ||
				cursor.ID != 50 {
				t.Fatalf("cursor = %#v", cursor)
			}

			return repository.AdminVideoListResult{
				Items: []repository.AdminVideoListItem{
					{
						VideoID:          40,
						AuthorID:         10,
						AuthorName:       "Alice",
						AuthorStatus:     entity.StatusActive,
						Title:            "first",
						Description:      "description",
						Category:         entity.CategoryBrewing,
						ProcessingStatus: entity.VideoProcessingReady,
						PublishStatus:    entity.VideoPublishPublished,
						CreatedAt:        firstCreatedAt,
						UpdatedAt:        firstCreatedAt,
					},
					{
						VideoID:          30,
						AuthorID:         11,
						AuthorName:       "Bob",
						AuthorStatus:     entity.StatusSuspended,
						Title:            "last",
						Description:      "description",
						Category:         entity.CategoryRoasting,
						ProcessingStatus: entity.VideoProcessingReady,
						PublishStatus:    entity.VideoPublishHidden,
						CreatedAt:        lastCreatedAt,
						UpdatedAt:        lastCreatedAt,
					},
				},
				HasMore: true,
			}, nil
		},
	}

	usecase := newAdminVideoUsecaseForTest(
		t,
		videos,
		&objectStorageRepositoryMock{},
	)

	result, err := usecase.List(
		context.Background(),
		activeAdminVideoActor(99),
		AdminVideoListInput{
			Limit: 2,
			Cursor: &AdminVideoCursor{
				CreatedAt: inputCursorTime,
				ID:        50,
			},
		},
	)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 2 || !result.HasMore || result.NextCursor == nil {
		t.Fatalf("result = %#v", result)
	}
	if result.Items[1].Author.Status != entity.StatusSuspended {
		t.Fatalf("last item = %#v", result.Items[1])
	}
	if result.Items[0].CreatedAt.Location() != time.UTC ||
		result.Items[1].UpdatedAt.Location() != time.UTC {
		t.Fatalf("items were not normalized to UTC: %#v", result.Items)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(*result.NextCursor)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	var cursor AdminVideoCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		t.Fatalf("decode cursor JSON: %v", err)
	}
	if cursor.ID != 30 ||
		!cursor.CreatedAt.Equal(lastCreatedAt) ||
		cursor.CreatedAt.Location() != time.UTC {
		t.Fatalf("cursor = %#v", cursor)
	}
}

func TestAdminVideoUsecaseListRejectsUnauthorizedActors(t *testing.T) {
	usecase := newAdminVideoUsecaseForTest(
		t,
		&adminVideoRepositoryMock{},
		&objectStorageRepositoryMock{},
	)

	tests := []struct {
		name  string
		actor *entity.User
		want  error
	}{
		{name: "nil actor", want: entity.ErrUnauthorized},
		{
			name: "suspended admin",
			actor: &entity.User{
				ID:     1,
				Role:   entity.RoleAdmin,
				Status: entity.StatusSuspended,
			},
			want: entity.ErrUnauthorized,
		},
		{
			name: "active user",
			actor: &entity.User{
				ID:     1,
				Role:   entity.RoleUser,
				Status: entity.StatusActive,
			},
			want: entity.ErrAdminRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := usecase.List(
				context.Background(),
				tt.actor,
				AdminVideoListInput{Limit: 20},
			)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAdminVideoUsecaseGetIssuesReadURLsForPublishedAndHidden(t *testing.T) {
	for _, publishStatus := range []entity.VideoPublishStatus{
		entity.VideoPublishPublished,
		entity.VideoPublishHidden,
	} {
		t.Run(string(publishStatus), func(t *testing.T) {
			videos := &adminVideoRepositoryMock{
				findByIDFunc: func(
					_ context.Context,
					videoID uint64,
				) (*repository.AdminVideoDetail, error) {
					if videoID != 10 {
						t.Fatalf("videoID = %d, want 10", videoID)
					}
					return &repository.AdminVideoDetail{
						VideoID:          10,
						AuthorID:         2,
						AuthorName:       "Alice",
						AuthorStatus:     entity.StatusActive,
						Title:            "title",
						Description:      "description",
						Category:         entity.CategoryBrewing,
						ProcessingStatus: entity.VideoProcessingReady,
						PublishStatus:    publishStatus,
						CreatedAt:        time.Now(),
						UpdatedAt:        time.Now(),
						OutputMeta: &repository.AdminVideoOutputMeta{
							VideoObjectKey:     "videos/10/output/video.mp4",
							ThumbnailObjectKey: "videos/10/thumbnail/image.jpg",
						},
					}, nil
				},
			}

			calledKeys := make([]string, 0, 2)
			storage := &objectStorageRepositoryMock{
				createReadURLFunc: func(
					_ context.Context,
					objectKey string,
					ttl time.Duration,
				) (repository.ReadTarget, error) {
					if ttl != 10*time.Minute {
						t.Fatalf("ttl = %s", ttl)
					}
					calledKeys = append(calledKeys, objectKey)
					return repository.ReadTarget{
						URL:       "https://storage.example/" + objectKey,
						ExpiresAt: time.Now().UTC().Add(ttl),
					}, nil
				},
			}

			usecase := newAdminVideoUsecaseForTest(t, videos, storage)
			result, err := usecase.Get(
				context.Background(),
				activeAdminVideoActor(99),
				10,
			)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if result.PlaybackURL == nil || result.ThumbnailURL == nil {
				t.Fatalf("result = %#v", result)
			}
			if len(calledKeys) != 2 ||
				calledKeys[0] != "videos/10/output/video.mp4" ||
				calledKeys[1] != "videos/10/thumbnail/image.jpg" {
				t.Fatalf("calledKeys = %#v", calledKeys)
			}
			if strings.Contains(*result.PlaybackURL, "thumbnail") ||
				strings.Contains(*result.ThumbnailURL, "output/video") {
				t.Fatalf("URLs were assigned incorrectly: %#v", result)
			}
		})
	}
}

func TestAdminVideoUsecaseGetDoesNotIssueReadURLsForUnavailableStates(t *testing.T) {
	tests := []struct {
		name             string
		processingStatus entity.VideoProcessingStatus
		publishStatus    entity.VideoPublishStatus
	}{
		{
			name:             "ready private",
			processingStatus: entity.VideoProcessingReady,
			publishStatus:    entity.VideoPublishPrivate,
		},
		{
			name:             "processing private",
			processingStatus: entity.VideoProcessingProcessing,
			publishStatus:    entity.VideoPublishPrivate,
		},
		{
			name:             "failed private",
			processingStatus: entity.VideoProcessingFailed,
			publishStatus:    entity.VideoPublishPrivate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videos := &adminVideoRepositoryMock{
				findByIDFunc: func(context.Context, uint64) (*repository.AdminVideoDetail, error) {
					return &repository.AdminVideoDetail{
						VideoID:          10,
						ProcessingStatus: tt.processingStatus,
						PublishStatus:    tt.publishStatus,
						OutputMeta: &repository.AdminVideoOutputMeta{
							VideoObjectKey:     "videos/10/output/video.mp4",
							ThumbnailObjectKey: "videos/10/thumbnail/image.jpg",
						},
					}, nil
				},
			}

			usecase := newAdminVideoUsecaseForTest(
				t,
				videos,
				&objectStorageRepositoryMock{},
			)
			result, err := usecase.Get(
				context.Background(),
				activeAdminVideoActor(99),
				10,
			)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if result.PlaybackURL != nil || result.ThumbnailURL != nil {
				t.Fatalf("result = %#v, want no read URLs", result)
			}
		})
	}
}

func TestAdminVideoUsecaseGetRejectsMissingOutputMeta(t *testing.T) {
	videos := &adminVideoRepositoryMock{
		findByIDFunc: func(context.Context, uint64) (*repository.AdminVideoDetail, error) {
			return &repository.AdminVideoDetail{
				VideoID:          10,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishPublished,
			}, nil
		},
	}

	usecase := newAdminVideoUsecaseForTest(
		t,
		videos,
		&objectStorageRepositoryMock{},
	)
	_, err := usecase.Get(
		context.Background(),
		activeAdminVideoActor(99),
		10,
	)
	if err == nil || !strings.Contains(err.Error(), "output metadata is missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdminVideoUsecaseHideNormalizesAuditInput(t *testing.T) {
	before := time.Now().UTC()
	updatedAt := before.Add(time.Minute)

	videos := &adminVideoRepositoryMock{
		hideFunc: func(
			_ context.Context,
			adminUserID uint64,
			videoID uint64,
			reason string,
			requestID string,
			now time.Time,
		) (*repository.AdminVideoState, error) {
			if adminUserID != 99 || videoID != 10 {
				t.Fatalf("IDs = %d, %d", adminUserID, videoID)
			}
			if reason != "規約違反" || requestID != "request-hide" {
				t.Fatalf("reason=%q requestID=%q", reason, requestID)
			}
			assertTimeNear(t, now, before, time.Now().UTC())
			return &repository.AdminVideoState{
				VideoID:          10,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishHidden,
				UpdatedAt:        updatedAt,
			}, nil
		},
	}

	usecase := newAdminVideoUsecaseForTest(
		t,
		videos,
		&objectStorageRepositoryMock{},
	)
	result, err := usecase.Hide(
		context.Background(),
		activeAdminVideoActor(99),
		10,
		"  規約違反  ",
		"  request-hide  ",
	)
	if err != nil {
		t.Fatalf("Hide() error = %v", err)
	}
	if result.ID != 10 ||
		result.ProcessingStatus != entity.VideoProcessingReady ||
		result.PublishStatus != entity.VideoPublishHidden ||
		!result.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdminVideoUsecaseRestorePropagatesOwnerSuspended(t *testing.T) {
	videos := &adminVideoRepositoryMock{
		restoreFunc: func(
			context.Context,
			uint64,
			uint64,
			string,
			string,
			time.Time,
		) (*repository.AdminVideoState, error) {
			return nil, entity.ErrUserSuspended
		},
	}

	usecase := newAdminVideoUsecaseForTest(
		t,
		videos,
		&objectStorageRepositoryMock{},
	)
	_, err := usecase.Restore(
		context.Background(),
		activeAdminVideoActor(99),
		10,
		"確認完了",
		"request-restore",
	)
	if !errors.Is(err, entity.ErrUserSuspended) {
		t.Fatalf("error = %v, want ErrUserSuspended", err)
	}
}

func TestAdminVideoUsecaseGetPropagatesStorageFailureWithoutObjectKey(t *testing.T) {
	const objectKey = "videos/10/output/private-object.mp4"

	videos := &adminVideoRepositoryMock{
		findByIDFunc: func(context.Context, uint64) (*repository.AdminVideoDetail, error) {
			return &repository.AdminVideoDetail{
				VideoID:          10,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishPublished,
				OutputMeta: &repository.AdminVideoOutputMeta{
					VideoObjectKey:     objectKey,
					ThumbnailObjectKey: "videos/10/thumbnail/private-object.jpg",
				},
			}, nil
		},
	}
	storage := &objectStorageRepositoryMock{
		createReadURLFunc: func(
			context.Context,
			string,
			time.Duration,
		) (repository.ReadTarget, error) {
			return repository.ReadTarget{}, errors.New("storage unavailable")
		},
	}

	adminVideos := newAdminVideoUsecaseForTest(t, videos, storage)
	_, err := adminVideos.Get(
		context.Background(),
		activeAdminVideoActor(99),
		10,
	)
	if err == nil {
		t.Fatal("Get() error = nil, want storage failure")
	}
	if strings.Contains(err.Error(), objectKey) {
		t.Fatalf("error exposed Object Key: %v", err)
	}
}

func TestAdminVideoUsecaseRestoreNormalizesAuditInput(t *testing.T) {
	before := time.Now().UTC()
	updatedAt := before.Add(time.Minute)

	videos := &adminVideoRepositoryMock{
		restoreFunc: func(
			_ context.Context,
			adminUserID uint64,
			videoID uint64,
			reason string,
			requestID string,
			now time.Time,
		) (*repository.AdminVideoState, error) {
			if adminUserID != 99 || videoID != 10 {
				t.Fatalf("IDs = %d, %d", adminUserID, videoID)
			}
			if reason != "再確認済み" || requestID != "request-restore" {
				t.Fatalf("reason=%q requestID=%q", reason, requestID)
			}
			assertTimeNear(t, now, before, time.Now().UTC())
			return &repository.AdminVideoState{
				VideoID:          10,
				ProcessingStatus: entity.VideoProcessingReady,
				PublishStatus:    entity.VideoPublishPublished,
				UpdatedAt:        updatedAt,
			}, nil
		},
	}

	adminVideos := newAdminVideoUsecaseForTest(
		t,
		videos,
		&objectStorageRepositoryMock{},
	)
	result, err := adminVideos.Restore(
		context.Background(),
		activeAdminVideoActor(99),
		10,
		"  再確認済み  ",
		"  request-restore  ",
	)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if result.ID != 10 ||
		result.ProcessingStatus != entity.VideoProcessingReady ||
		result.PublishStatus != entity.VideoPublishPublished ||
		!result.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("result = %#v", result)
	}
}
