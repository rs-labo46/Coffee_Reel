package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

func validVideoUsecaseConfig() VideoUsecaseConfig {
	return VideoUsecaseConfig{
		UploadURLTTL:       15 * time.Minute,
		ReadURLTTL:         10 * time.Minute,
		IdempotencyTTL:     24 * time.Hour,
		IdempotencyHMACKey: []byte("01234567890123456789012345678901"),
		ManagedPrefix:      "videos/",
	}
}

func TestNewVideoUsecaseRejectsInvalidConfiguration(t *testing.T) {
	validVideos := &videoRepositoryMock{}
	validStorage := &objectStorageRepositoryMock{}

	tests := []struct {
		name    string
		videos  repository.IVideoRepository
		storage repository.IObjectStorageRepository
		mutate  func(*VideoUsecaseConfig)
	}{
		{name: "video repository missing", storage: validStorage},
		{name: "storage repository missing", videos: validVideos},
		{name: "upload ttl is zero", videos: validVideos, storage: validStorage, mutate: func(c *VideoUsecaseConfig) { c.UploadURLTTL = 0 }},
		{name: "read ttl is zero", videos: validVideos, storage: validStorage, mutate: func(c *VideoUsecaseConfig) { c.ReadURLTTL = 0 }},
		{name: "idempotency ttl is zero", videos: validVideos, storage: validStorage, mutate: func(c *VideoUsecaseConfig) { c.IdempotencyTTL = 0 }},
		{name: "hmac key missing", videos: validVideos, storage: validStorage, mutate: func(c *VideoUsecaseConfig) { c.IdempotencyHMACKey = nil }},
		{name: "managed prefix outside videos", videos: validVideos, storage: validStorage, mutate: func(c *VideoUsecaseConfig) { c.ManagedPrefix = "other/" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validVideoUsecaseConfig()
			if tt.mutate != nil {
				tt.mutate(&config)
			}
			if _, err := NewVideoUsecase(tt.videos, tt.storage, config); err == nil {
				t.Fatal("NewVideoUsecase() error = nil, want configuration error")
			}
		})
	}
}

func TestVideoUsecaseStartUploadGeneratesServerManagedSourceObjectKey(t *testing.T) {
	const videoID uint64 = 42
	var storedVideo *entity.Video

	videos := &videoRepositoryMock{
		createWithIdempotencyFunc: func(_ context.Context, video *entity.Video, record *entity.IdempotencyRecord) (repository.VideoCreateResult, error) {
			if record == nil || record.KeyHash == "" || record.RequestHash == "" {
				t.Fatal("idempotency hashes were not generated")
			}
			if strings.Contains(record.KeyHash, "plain-key") {
				t.Fatal("plaintext idempotency key leaked into KeyHash")
			}
			video.ID = videoID
			storedVideo = video
			return repository.VideoCreateResult{Video: video, Created: true}, nil
		},
	}
	storage := &objectStorageRepositoryMock{
		createUploadURLFunc: func(_ context.Context, objectKey, contentType string, ttl time.Duration) (repository.UploadTarget, error) {
			if !regexp.MustCompile(`^videos/source/[0-9a-f]{32}\.mp4$`).MatchString(objectKey) {
				t.Fatalf("objectKey = %q, want videos/source/{32 hex chars}.mp4", objectKey)
			}
			if contentType != "video/mp4" {
				t.Fatalf("contentType = %q, want video/mp4", contentType)
			}
			if ttl <= 0 || ttl > 15*time.Minute {
				t.Fatalf("ttl = %s, want positive and at most 15m", ttl)
			}
			return repository.UploadTarget{Method: "PUT", URL: "https://upload.example", ContentType: contentType, ExpiresAt: time.Now().Add(ttl)}, nil
		},
	}
	uc, err := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())
	if err != nil {
		t.Fatalf("NewVideoUsecase() error = %v", err)
	}

	result, err := uc.StartUpload(context.Background(), activeVideoUser(7), StartUploadInput{
		Title: "抽出動画", Category: entity.CategoryBrewing, ContentType: "video/mp4", DeclaredSize: 1_000,
	}, "plain-key")
	if err != nil {
		t.Fatalf("StartUpload() error = %v", err)
	}
	if storedVideo == nil {
		t.Fatal("video was not passed to repository")
	}

	pattern := regexp.MustCompile(`^videos/source/[0-9a-f]{32}\.mp4$`)
	if !pattern.MatchString(storedVideo.OriginalObjectKey) {
		t.Fatalf("OriginalObjectKey = %q, want videos/source/{32 hex chars}.mp4", storedVideo.OriginalObjectKey)
	}
	if strings.Contains(storedVideo.OriginalObjectKey, "抽出動画") {
		t.Fatal("user supplied title leaked into object key")
	}
	if result.VideoID != videoID || !result.Created {
		t.Fatalf("result = %+v, want VideoID=%d and Created=true", result, videoID)
	}
}

func TestVideoUsecaseStartUploadReusesExistingUploadingVideoWithoutExtendingDeadline(t *testing.T) {
	now := time.Now()
	existing := &entity.Video{
		ID: 9, UserID: 3, OriginalObjectKey: "videos/9/source/existing.mp4",
		UploadExpiresAt: now.Add(4 * time.Minute), ProcessingStatus: entity.VideoProcessingUploading,
		PublishStatus: entity.VideoPublishPrivate, CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	}
	var gotTTL time.Duration
	videos := &videoRepositoryMock{createWithIdempotencyFunc: func(_ context.Context, _ *entity.Video, _ *entity.IdempotencyRecord) (repository.VideoCreateResult, error) {
		return repository.VideoCreateResult{Video: existing, Created: false}, nil
	}}
	storage := &objectStorageRepositoryMock{createUploadURLFunc: func(_ context.Context, key, contentType string, ttl time.Duration) (repository.UploadTarget, error) {
		if key != existing.OriginalObjectKey {
			t.Fatalf("objectKey = %q, want existing key %q", key, existing.OriginalObjectKey)
		}
		gotTTL = ttl
		return repository.UploadTarget{Method: "PUT", URL: "https://upload.example", ContentType: contentType, ExpiresAt: time.Now().Add(ttl)}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	result, err := uc.StartUpload(context.Background(), activeVideoUser(3), StartUploadInput{
		Title: "再送", Category: entity.CategoryBrewing, ContentType: "video/mp4", DeclaredSize: 100,
	}, "same-key")
	if err != nil {
		t.Fatalf("StartUpload() error = %v", err)
	}
	if result.Created {
		t.Fatal("Created = true, want false for idempotent replay")
	}
	if gotTTL <= 0 || gotTTL > 4*time.Minute {
		t.Fatalf("upload ttl = %s, want remaining original deadline only", gotTTL)
	}
	if !result.UploadExpiresAt.Equal(existing.UploadExpiresAt) {
		t.Fatalf("UploadExpiresAt = %s, want %s", result.UploadExpiresAt, existing.UploadExpiresAt)
	}
}

func TestVideoUsecaseStartUploadPropagatesIdempotencyConflictAndDoesNotPresign(t *testing.T) {
	storageCalled := false
	videos := &videoRepositoryMock{createWithIdempotencyFunc: func(context.Context, *entity.Video, *entity.IdempotencyRecord) (repository.VideoCreateResult, error) {
		return repository.VideoCreateResult{}, entity.ErrIdempotencyConflict
	}}
	storage := &objectStorageRepositoryMock{createUploadURLFunc: func(context.Context, string, string, time.Duration) (repository.UploadTarget, error) {
		storageCalled = true
		return repository.UploadTarget{}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	_, err := uc.StartUpload(context.Background(), activeVideoUser(1), StartUploadInput{
		Title: "A", Category: entity.CategoryBrewing, ContentType: "video/mp4", DeclaredSize: 1,
	}, "reused-key")
	if !errors.Is(err, entity.ErrIdempotencyConflict) {
		t.Fatalf("error = %v, want ErrIdempotencyConflict", err)
	}
	if storageCalled {
		t.Fatal("CreateUploadURL was called after idempotency conflict")
	}
}

func TestVideoUsecaseCompleteUploadRejectsExpiredBeforeStorageInspection(t *testing.T) {
	now := time.Now()
	expired := &entity.Video{
		ID: 12, UserID: 5, OriginalObjectKey: "videos/source/a.mp4",
		UploadExpiresAt: now.Add(-time.Minute), ProcessingStatus: entity.VideoProcessingUploading,
		PublishStatus: entity.VideoPublishPrivate,
	}
	statCalled := false
	completeCalled := false
	videos := &videoRepositoryMock{
		findOwnedByIDFunc: func(context.Context, uint64, uint64) (*repository.OwnedVideoDetail, error) {
			return &repository.OwnedVideoDetail{Video: expired}, nil
		},
		completeUploadFunc: func(context.Context, uint64, uint64, time.Time) (*entity.Video, error) {
			completeCalled = true
			return expired, nil
		},
	}
	storage := &objectStorageRepositoryMock{statFunc: func(context.Context, string) (repository.StoredObjectInfo, error) {
		statCalled = true
		return repository.StoredObjectInfo{}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	_, err := uc.CompleteUpload(context.Background(), activeVideoUser(5), expired.ID)
	if !errors.Is(err, entity.ErrUploadExpired) {
		t.Fatalf("error = %v, want ErrUploadExpired", err)
	}
	if statCalled {
		t.Fatal("Storage.Stat was called for an expired upload")
	}
	if completeCalled {
		t.Fatal("repository CompleteUpload was called for an expired upload")
	}
}

func TestVideoUsecaseCompleteUploadIsIdempotentAfterUploadCompleted(t *testing.T) {
	statuses := []entity.VideoProcessingStatus{
		entity.VideoProcessingUploaded,
		entity.VideoProcessingProcessing,
		entity.VideoProcessingReady,
		entity.VideoProcessingFailed,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			video := &entity.Video{ID: 7, UserID: 2, ProcessingStatus: status, PublishStatus: entity.VideoPublishPrivate, UpdatedAt: time.Now()}
			videos := &videoRepositoryMock{findOwnedByIDFunc: func(context.Context, uint64, uint64) (*repository.OwnedVideoDetail, error) {
				return &repository.OwnedVideoDetail{Video: video}, nil
			}}
			uc, _ := NewVideoUsecase(videos, &objectStorageRepositoryMock{}, validVideoUsecaseConfig())

			result, err := uc.CompleteUpload(context.Background(), activeVideoUser(2), 7)
			if err != nil {
				t.Fatalf("CompleteUpload() error = %v", err)
			}
			if result.ProcessingStatus != status {
				t.Fatalf("status = %s, want %s", result.ProcessingStatus, status)
			}
		})
	}
}

func TestVideoUsecaseCompleteUploadRejectsMissingOrInvalidStoredObject(t *testing.T) {
	now := time.Now()
	video := &entity.Video{ID: 7, UserID: 2, OriginalObjectKey: "videos/7/source/x.mp4", UploadExpiresAt: now.Add(time.Minute), ProcessingStatus: entity.VideoProcessingUploading, PublishStatus: entity.VideoPublishPrivate}

	tests := []struct {
		name string
		info repository.StoredObjectInfo
		err  error
	}{
		{name: "missing", err: entity.ErrObjectNotFound},
		{name: "empty", info: repository.StoredObjectInfo{SizeBytes: 0, ContentType: "video/mp4"}},
		{name: "too large", info: repository.StoredObjectInfo{SizeBytes: maxVideoSizeBytes + 1, ContentType: "video/mp4"}},
		{name: "unsupported content type", info: repository.StoredObjectInfo{SizeBytes: 100, ContentType: "video/webm"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completeCalled := false
			videos := &videoRepositoryMock{
				findOwnedByIDFunc: func(context.Context, uint64, uint64) (*repository.OwnedVideoDetail, error) {
					return &repository.OwnedVideoDetail{Video: video}, nil
				},
				completeUploadFunc: func(context.Context, uint64, uint64, time.Time) (*entity.Video, error) {
					completeCalled = true
					return video, nil
				},
			}
			storage := &objectStorageRepositoryMock{statFunc: func(context.Context, string) (repository.StoredObjectInfo, error) { return tt.info, tt.err }}
			uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

			_, err := uc.CompleteUpload(context.Background(), activeVideoUser(2), 7)
			if !errors.Is(err, entity.ErrVideoSourceInvalid) {
				t.Fatalf("error = %v, want ErrVideoSourceInvalid", err)
			}
			if completeCalled {
				t.Fatal("CompleteUpload repository method was called for invalid source")
			}
		})
	}
}

func TestVideoUsecaseListReelsAllBuildsReadURLsStateAndCursor(t *testing.T) {
	createdAt := time.Date(2026, 8, 8, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, input repository.PublicVideoListInput) (repository.PublicVideoPage, error) {
		if input.Limit != 2 || input.Cursor != nil || input.ViewerUserID != nil || input.SearchMode != repository.PublicVideoSearchAll {
			t.Fatalf("ListPublic input = %#v", input)
		}
		if input.Title != "" || input.Category != nil {
			t.Fatalf("unexpected filters: %#v", input)
		}
		return repository.PublicVideoPage{
			Items: []repository.PublicVideoItem{{
				ID: 11, UserID: 4, AuthorName: "author", Category: entity.CategoryBrewing,
				Title: "title", Description: "description",
				VideoObjectKey: "videos/11/output/a.mp4", ThumbnailObjectKey: "videos/11/thumbnail/a.jpg",
				CreatedAt: createdAt, LikeCount: 3, IsLiked: false, IsSaved: false,
			}},
			HasMore: true, LastCreatedAt: createdAt, LastID: 11,
		}, nil
	}}
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(_ context.Context, key string, ttl time.Duration) (repository.ReadTarget, error) {
		if ttl != 10*time.Minute {
			t.Fatalf("ttl = %s, want 10m", ttl)
		}
		return repository.ReadTarget{URL: "https://read.example/" + key, ExpiresAt: time.Now().Add(ttl)}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	result, err := uc.ListReels(context.Background(), nil, PublicVideoListInput{Limit: 2})
	if err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if result.ResultType != PublicSearchAll || len(result.Items) != 1 || !result.HasMore || result.NextCursor == nil {
		t.Fatalf("result = %+v", result)
	}
	item := result.Items[0]
	if item.LikeCount != 3 || item.IsLiked || item.IsSaved {
		t.Fatalf("public state = %+v", item)
	}
	if item.Video.URL == "" || item.Thumbnail.URL == "" || strings.Contains(item.Video.URL, "ObjectKey") {
		t.Fatalf("read targets = %+v", item)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(*result.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	var cursor PublicVideoCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	wantHash, err := publicVideoFilterHash("", nil)
	if err != nil {
		t.Fatalf("publicVideoFilterHash() error = %v", err)
	}
	if cursor.ResultType != PublicSearchAll || cursor.Similarity != 0 || cursor.ID != 11 ||
		!cursor.CreatedAt.Equal(createdAt) || cursor.FilterHash != wantHash {
		t.Fatalf("cursor = %+v", cursor)
	}
}

func TestVideoUsecaseListReelsMatchedKeepsFiltersAndViewer(t *testing.T) {
	category := entity.CategoryBrewing
	viewer := activeVideoUser(9)
	calls := 0
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, input repository.PublicVideoListInput) (repository.PublicVideoPage, error) {
		calls++
		if input.SearchMode != repository.PublicVideoSearchMatched || input.Title != "latte" || input.Category == nil || *input.Category != category {
			t.Fatalf("ListPublic input = %#v", input)
		}
		if input.ViewerUserID == nil || *input.ViewerUserID != viewer.ID {
			t.Fatalf("ViewerUserID = %v, want %d", input.ViewerUserID, viewer.ID)
		}
		return repository.PublicVideoPage{Items: []repository.PublicVideoItem{{
			ID: 1, UserID: 2, AuthorName: "author", Category: category, Title: "Latte Art",
			VideoObjectKey: "videos/1/output/a.mp4", ThumbnailObjectKey: "videos/1/thumbnail/a.jpg",
			CreatedAt: time.Now(), LikeCount: 4, IsLiked: true, IsSaved: true,
		}}}, nil
	}}
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(_ context.Context, key string, ttl time.Duration) (repository.ReadTarget, error) {
		return repository.ReadTarget{URL: "https://read.example/" + key, ExpiresAt: time.Now().Add(ttl)}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	result, err := uc.ListReels(context.Background(), viewer, PublicVideoListInput{Title: "latte", Category: &category, Limit: 20})
	if err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if calls != 1 || result.ResultType != PublicSearchMatched || len(result.Items) != 1 {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
	if !result.Items[0].IsLiked || !result.Items[0].IsSaved || result.Items[0].LikeCount != 4 {
		t.Fatalf("public state = %+v", result.Items[0])
	}
}

func TestVideoUsecaseListReelsFallsBackToSimilarOnlyWhenTitlePresent(t *testing.T) {
	category := entity.CategoryLatteArt
	createdAt := time.Date(2026, 8, 8, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	calls := 0
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, input repository.PublicVideoListInput) (repository.PublicVideoPage, error) {
		calls++
		if input.Title != "latte" || input.Category == nil || *input.Category != category {
			t.Fatalf("filters changed during fallback: %#v", input)
		}
		switch calls {
		case 1:
			if input.SearchMode != repository.PublicVideoSearchMatched {
				t.Fatalf("first SearchMode = %s", input.SearchMode)
			}
			return repository.PublicVideoPage{}, nil
		case 2:
			if input.SearchMode != repository.PublicVideoSearchSimilar {
				t.Fatalf("second SearchMode = %s", input.SearchMode)
			}
			return repository.PublicVideoPage{
				Items: []repository.PublicVideoItem{{
					ID: 7, UserID: 3, AuthorName: "author", Category: category, Title: "latte art",
					VideoObjectKey: "videos/7/output/a.mp4", ThumbnailObjectKey: "videos/7/thumbnail/a.jpg",
					CreatedAt: createdAt, Similarity: 0.75,
				}},
				HasMore: true, LastCreatedAt: createdAt, LastID: 7, LastSimilarity: 0.75,
			}, nil
		default:
			t.Fatalf("unexpected ListPublic call %d", calls)
			return repository.PublicVideoPage{}, nil
		}
	}}
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(_ context.Context, key string, ttl time.Duration) (repository.ReadTarget, error) {
		return repository.ReadTarget{URL: "https://read.example/" + key, ExpiresAt: time.Now().Add(ttl)}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	result, err := uc.ListReels(context.Background(), nil, PublicVideoListInput{Title: "latte", Category: &category, Limit: 10})
	if err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if calls != 2 || result.ResultType != PublicSearchSimilar || len(result.Items) != 1 || result.NextCursor == nil {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*result.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	var cursor PublicVideoCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	if cursor.ResultType != PublicSearchSimilar || cursor.Similarity != 0.75 {
		t.Fatalf("cursor = %+v", cursor)
	}
}

func TestVideoUsecaseListReelsCategoryOnlyZeroDoesNotFallback(t *testing.T) {
	category := entity.CategoryBeans
	calls := 0
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, input repository.PublicVideoListInput) (repository.PublicVideoPage, error) {
		calls++
		if input.SearchMode != repository.PublicVideoSearchMatched || input.Title != "" || input.Category == nil || *input.Category != category {
			t.Fatalf("ListPublic input = %#v", input)
		}
		return repository.PublicVideoPage{}, nil
	}}
	uc, _ := NewVideoUsecase(videos, &objectStorageRepositoryMock{}, validVideoUsecaseConfig())

	result, err := uc.ListReels(context.Background(), nil, PublicVideoListInput{Category: &category, Limit: 20})
	if err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if calls != 1 || result.ResultType != PublicSearchMatched || len(result.Items) != 0 {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
}

func TestVideoUsecaseListReelsSimilarFallbackCanReturnEmpty(t *testing.T) {
	calls := 0
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, input repository.PublicVideoListInput) (repository.PublicVideoPage, error) {
		calls++
		if calls == 1 && input.SearchMode != repository.PublicVideoSearchMatched {
			t.Fatalf("first SearchMode = %s", input.SearchMode)
		}
		if calls == 2 && input.SearchMode != repository.PublicVideoSearchSimilar {
			t.Fatalf("second SearchMode = %s", input.SearchMode)
		}
		return repository.PublicVideoPage{}, nil
	}}
	uc, _ := NewVideoUsecase(videos, &objectStorageRepositoryMock{}, validVideoUsecaseConfig())

	result, err := uc.ListReels(context.Background(), nil, PublicVideoListInput{Title: "nothing", Limit: 20})
	if err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if calls != 2 || result.ResultType != PublicSearchSimilar || len(result.Items) != 0 || result.HasMore || result.NextCursor != nil {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
}

func TestVideoUsecaseListReelsRejectsCursorWithDifferentFilterHash(t *testing.T) {
	repositoryCalled := false
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, input repository.PublicVideoListInput) (repository.PublicVideoPage, error) {
		repositoryCalled = true
		return repository.PublicVideoPage{}, nil
	}}
	uc, _ := NewVideoUsecase(videos, &objectStorageRepositoryMock{}, validVideoUsecaseConfig())

	_, err := uc.ListReels(context.Background(), nil, PublicVideoListInput{
		Title: "latte",
		Limit: 20,
		Cursor: &PublicVideoCursor{
			ResultType: PublicSearchMatched,
			CreatedAt:  time.Now(),
			ID:         1,
			FilterHash: strings.Repeat("0", 64),
		},
	})
	if !errors.Is(err, entity.ErrCursorInvalid) {
		t.Fatalf("error = %v, want ErrCursorInvalid", err)
	}
	if repositoryCalled {
		t.Fatal("repository was called with a cursor from different filters")
	}
}

func TestVideoUsecaseGetDetailMapsLikeSavedStateAndReadURLs(t *testing.T) {
	createdAt := time.Date(2026, 8, 8, 13, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	viewer := activeVideoUser(10)
	videos := &videoRepositoryMock{
		findPublicByIDFunc: func(_ context.Context, videoID uint64, viewerID *uint64) (*repository.PublicVideoItem, error) {
			if videoID != 44 || viewerID == nil || *viewerID != viewer.ID {
				t.Fatalf("FindPublicByID args videoID=%d viewerID=%v", videoID, viewerID)
			}
			return &repository.PublicVideoItem{
				ID:                 44,
				UserID:             3,
				AuthorName:         "author",
				Category:           entity.CategoryLatteArt,
				Title:              "latte art",
				Description:        "desc",
				VideoObjectKey:     "videos/44/output/video.mp4",
				ThumbnailObjectKey: "videos/44/thumbnail/thumb.jpg",
				CreatedAt:          createdAt,
				LikeCount:          7,
				IsLiked:            true,
				IsSaved:            true,
			}, nil
		},
	}
	storage := &objectStorageRepositoryMock{
		createReadURLFunc: func(_ context.Context, objectKey string, ttl time.Duration) (repository.ReadTarget, error) {
			if ttl != 10*time.Minute {
				t.Fatalf("ttl = %s, want 10m", ttl)
			}
			return repository.ReadTarget{URL: "https://read.example/" + objectKey, ExpiresAt: time.Now().Add(ttl)}, nil
		},
	}
	uc, err := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())
	if err != nil {
		t.Fatalf("NewVideoUsecase() error = %v", err)
	}

	result, err := uc.GetDetail(context.Background(), viewer, 44)
	if err != nil {
		t.Fatalf("GetDetail() error = %v", err)
	}
	if result.ID != 44 || result.LikeCount != 7 || !result.IsLiked || !result.IsSaved {
		t.Fatalf("result = %+v", result)
	}
	if result.Video.URL == "" || result.Thumbnail.URL == "" {
		t.Fatalf("read URLs were not issued: %+v", result)
	}
}

func TestVideoUsecaseGetMineDoesNotIssueReadURLForHiddenVideo(t *testing.T) {
	video := &entity.Video{ID: 8, UserID: 2, ProcessingStatus: entity.VideoProcessingReady, PublishStatus: entity.VideoPublishHidden, UploadExpiresAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	videos := &videoRepositoryMock{findOwnedByIDFunc: func(context.Context, uint64, uint64) (*repository.OwnedVideoDetail, error) {
		return &repository.OwnedVideoDetail{Video: video, OutputMeta: &entity.OutputVideoMeta{VideoObjectKey: "videos/8/output/a.mp4", ThumbnailObjectKey: "videos/8/thumbnail/a.jpg"}}, nil
	}}
	readCalled := false
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(context.Context, string, time.Duration) (repository.ReadTarget, error) {
		readCalled = true
		return repository.ReadTarget{}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	result, err := uc.GetMine(context.Background(), activeVideoUser(2), 8)
	if err != nil {
		t.Fatalf("GetMine() error = %v", err)
	}
	if readCalled || result.OutputMeta != nil {
		t.Fatalf("hidden video received read information: %+v", result.OutputMeta)
	}
}

func TestVideoUsecaseOwnerOperationsValidateActorAndForwardRepositoryErrors(t *testing.T) {
	uc, _ := NewVideoUsecase(&videoRepositoryMock{}, &objectStorageRepositoryMock{}, validVideoUsecaseConfig())
	if _, err := uc.SetPrivate(context.Background(), suspendedVideoUser(1), 2); !errors.Is(err, entity.ErrUserSuspended) {
		t.Fatalf("SetPrivate error = %v", err)
	}
	if _, err := uc.Republish(context.Background(), activeVideoUser(1), 0); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("Republish error = %v", err)
	}
	if err := uc.Delete(context.Background(), nil, 2); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("Delete error = %v", err)
	}

	want := entity.ErrVideoStateConflict
	videos := &videoRepositoryMock{republishByOwnerFunc: func(context.Context, uint64, uint64, time.Time) (*entity.Video, error) { return nil, want }}
	uc, _ = NewVideoUsecase(videos, &objectStorageRepositoryMock{}, validVideoUsecaseConfig())
	if _, err := uc.Republish(context.Background(), activeVideoUser(1), 2); !errors.Is(err, want) {
		t.Fatalf("Republish error = %v, want %v", err, want)
	}
}
