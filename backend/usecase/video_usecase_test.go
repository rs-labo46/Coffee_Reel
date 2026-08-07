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

func TestVideoUsecaseListReelsBuildsReadURLsAndOpaqueCursor(t *testing.T) {
	createdAt := time.Date(2026, 8, 3, 1, 2, 3, 0, time.FixedZone("JST", 9*60*60))
	videos := &videoRepositoryMock{listPublicFunc: func(_ context.Context, limit int, cursor *repository.VideoCursor, viewerID *uint64) (repository.PublicVideoPage, error) {
		if limit != 20 || cursor != nil || viewerID != nil {
			t.Fatalf("ListPublic(limit=%d, cursor=%v, viewerID=%v)", limit, cursor, viewerID)
		}
		return repository.PublicVideoPage{
			Items:   []repository.PublicVideoItem{{ID: 11, UserID: 4, AuthorName: "author", Category: entity.CategoryBrewing, Title: "title", VideoObjectKey: "videos/11/output/a.mp4", ThumbnailObjectKey: "videos/11/thumbnail/a.jpg", CreatedAt: createdAt}},
			HasMore: true, LastCreatedAt: createdAt, LastID: 11,
		}, nil
	}}
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(_ context.Context, key string, ttl time.Duration) (repository.ReadTarget, error) {
		if ttl != 10*time.Minute {
			t.Fatalf("ttl = %s", ttl)
		}
		return repository.ReadTarget{URL: "https://read.example/" + key, ExpiresAt: time.Now().Add(ttl)}, nil
	}}
	uc, _ := NewVideoUsecase(videos, storage, validVideoUsecaseConfig())

	result, err := uc.ListReels(context.Background(), nil, VideoListInput{Limit: 20})
	if err != nil {
		t.Fatalf("ListReels() error = %v", err)
	}
	if len(result.Items) != 1 || result.NextCursor == nil || !result.HasMore {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Items[0].Video.URL, "ObjectKey") || result.Items[0].Video.URL == "" || result.Items[0].Thumbnail.URL == "" {
		t.Fatalf("read targets = %+v", result.Items[0])
	}

	decoded, err := base64.RawURLEncoding.DecodeString(*result.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	var cursor VideoCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	if cursor.ID != 11 || cursor.CreatedAt.Location() != time.UTC || !cursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("cursor = %+v", cursor)
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
