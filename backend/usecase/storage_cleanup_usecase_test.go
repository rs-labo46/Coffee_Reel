package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

func validStorageCleanupConfig() StorageCleanupUsecaseConfig {
	return StorageCleanupUsecaseConfig{
		RetryDelays:  [3]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute},
		Timeout:      2 * time.Minute,
		OrphanMinAge: time.Hour,
		ListLimit:    100,
	}
}

func runningCleanupJob(attempt int) *entity.StorageCleanupJob {
	now := time.Now()
	started := now.Add(-time.Second)
	return &entity.StorageCleanupJob{
		ID: 41, ObjectKey: "videos/22/output/file.mp4", AssetType: entity.StorageAssetProcessed,
		Cause: entity.StorageCleanupRollback, Status: entity.StorageCleanupRunning,
		AttemptCount: attempt, MaxAttempts: 4, AvailableAt: now, StartedAt: &started,
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestNewStorageCleanupUsecaseRejectsInvalidConfiguration(t *testing.T) {
	validJobs := &storageCleanupJobRepositoryMock{}
	validStorage := &objectStorageRepositoryMock{}
	validVideos := &videoRepositoryMock{}
	tests := []struct {
		name    string
		jobs    repository.IStorageCleanupJobRepository
		storage repository.IObjectStorageRepository
		videos  repository.IVideoRepository
		mutate  func(*StorageCleanupUsecaseConfig)
	}{
		{name: "jobs missing", storage: validStorage, videos: validVideos},
		{name: "storage missing", jobs: validJobs, videos: validVideos},
		{name: "videos missing", jobs: validJobs, storage: validStorage},
		{name: "timeout zero", jobs: validJobs, storage: validStorage, videos: validVideos, mutate: func(c *StorageCleanupUsecaseConfig) { c.Timeout = 0 }},
		{name: "orphan age zero", jobs: validJobs, storage: validStorage, videos: validVideos, mutate: func(c *StorageCleanupUsecaseConfig) { c.OrphanMinAge = 0 }},
		{name: "list limit zero", jobs: validJobs, storage: validStorage, videos: validVideos, mutate: func(c *StorageCleanupUsecaseConfig) { c.ListLimit = 0 }},
		{name: "list limit over max", jobs: validJobs, storage: validStorage, videos: validVideos, mutate: func(c *StorageCleanupUsecaseConfig) { c.ListLimit = 1001 }},
		{name: "retry delay zero", jobs: validJobs, storage: validStorage, videos: validVideos, mutate: func(c *StorageCleanupUsecaseConfig) { c.RetryDelays[2] = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validStorageCleanupConfig()
			if tt.mutate != nil {
				tt.mutate(&config)
			}
			if _, err := NewStorageCleanupUsecase(tt.jobs, tt.storage, tt.videos, config); err == nil {
				t.Fatal("NewStorageCleanupUsecase() error = nil")
			}
		})
	}
}

func TestStorageCleanupUsecaseProcessNextReturnsNoWorkWhenClaimIsNil(t *testing.T) {
	jobs := &storageCleanupJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*entity.StorageCleanupJob, error) { return nil, nil }}
	uc, _ := NewStorageCleanupUsecase(jobs, &objectStorageRepositoryMock{}, &videoRepositoryMock{}, validStorageCleanupConfig())
	processed, err := uc.ProcessNext(context.Background())
	if err != nil || processed {
		t.Fatalf("ProcessNext() = (%v, %v), want (false, nil)", processed, err)
	}
}

func TestStorageCleanupUsecaseProcessNextRejectsInvalidClaim(t *testing.T) {
	job := runningCleanupJob(1)
	job.ObjectKey = "   "
	jobs := &storageCleanupJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*entity.StorageCleanupJob, error) { return job, nil }}
	uc, _ := NewStorageCleanupUsecase(jobs, &objectStorageRepositoryMock{}, &videoRepositoryMock{}, validStorageCleanupConfig())
	processed, err := uc.ProcessNext(context.Background())
	if !processed || !errors.Is(err, entity.ErrCleanupJobConflict) {
		t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
	}
}

func TestStorageCleanupUsecaseObjectNotFoundIsMarkedSucceeded(t *testing.T) {
	job := runningCleanupJob(1)
	marked := false
	jobs := &storageCleanupJobRepositoryMock{
		claimNextFunc: func(context.Context, time.Time) (*entity.StorageCleanupJob, error) { return job, nil },
		markSucceededFunc: func(_ context.Context, jobID uint64, now time.Time) error {
			marked = true
			if jobID != job.ID || now.Location() != time.UTC {
				t.Fatalf("MarkSucceeded(%d, %s)", jobID, now)
			}
			return nil
		},
	}
	storage := &objectStorageRepositoryMock{deleteFunc: func(context.Context, string) error { return entity.ErrObjectNotFound }}
	uc, _ := NewStorageCleanupUsecase(jobs, storage, &videoRepositoryMock{}, validStorageCleanupConfig())
	processed, err := uc.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
	}
	if !marked {
		t.Fatal("MarkSucceeded was not called for missing object")
	}
}

func TestStorageCleanupUsecaseTemporaryFailureSchedulesRetry(t *testing.T) {
	job := runningCleanupJob(1)
	var availableAt, scheduledAt time.Time
	jobs := &storageCleanupJobRepositoryMock{
		claimNextFunc: func(context.Context, time.Time) (*entity.StorageCleanupJob, error) { return job, nil },
		scheduleRetryFunc: func(_ context.Context, jobID uint64, available time.Time, code, message string, now time.Time) error {
			availableAt, scheduledAt = available, now
			if jobID != job.ID || code != storageCleanupTemporaryCode || message != storageCleanupTemporaryMessage {
				t.Fatalf("ScheduleRetry(%d, %q, %q)", jobID, code, message)
			}
			return nil
		},
	}
	storage := &objectStorageRepositoryMock{deleteFunc: func(context.Context, string) error { return entity.ErrStorageUnavailable }}
	uc, _ := NewStorageCleanupUsecase(jobs, storage, &videoRepositoryMock{}, validStorageCleanupConfig())
	processed, err := uc.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
	}
	if availableAt.Sub(scheduledAt) != time.Minute {
		t.Fatalf("retry delay = %s, want 1m", availableAt.Sub(scheduledAt))
	}
}

func TestStorageCleanupUsecaseDeleteTimeoutSchedulesRetry(t *testing.T) {
	job := runningCleanupJob(1)
	scheduled := false
	jobs := &storageCleanupJobRepositoryMock{
		claimNextFunc: func(context.Context, time.Time) (*entity.StorageCleanupJob, error) { return job, nil },
		scheduleRetryFunc: func(_ context.Context, _ uint64, _ time.Time, code, message string, _ time.Time) error {
			scheduled = true
			if code != storageCleanupTemporaryCode || message != storageCleanupTimeoutMessage {
				t.Fatalf("code=%q message=%q", code, message)
			}
			return nil
		},
	}
	storage := &objectStorageRepositoryMock{deleteFunc: func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	config := validStorageCleanupConfig()
	config.Timeout = 5 * time.Millisecond
	uc, _ := NewStorageCleanupUsecase(jobs, storage, &videoRepositoryMock{}, config)
	processed, err := uc.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
	}
	if !scheduled {
		t.Fatal("ScheduleRetry was not called after timeout")
	}
}

func TestStorageCleanupUsecasePermanentOrFinalAttemptFailureIsMarkedFailed(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		deleteErr   error
		wantCode    string
		wantMessage string
	}{
		{name: "permanent first attempt", attempt: 1, deleteErr: errors.New("permission denied"), wantCode: storageCleanupPermanentCode, wantMessage: storageCleanupPermanentMessage},
		{name: "temporary fourth attempt", attempt: 4, deleteErr: entity.ErrStorageUnavailable, wantCode: storageCleanupTemporaryCode, wantMessage: storageCleanupTemporaryMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := runningCleanupJob(tt.attempt)
			marked := false
			jobs := &storageCleanupJobRepositoryMock{
				claimNextFunc: func(context.Context, time.Time) (*entity.StorageCleanupJob, error) { return job, nil },
				markFailedFunc: func(_ context.Context, jobID uint64, code, message string, now time.Time) error {
					marked = true
					if jobID != job.ID || code != tt.wantCode || message != tt.wantMessage || now.Location() != time.UTC {
						t.Fatalf("MarkFailed(%d,%q,%q,%s)", jobID, code, message, now)
					}
					return nil
				},
			}
			storage := &objectStorageRepositoryMock{deleteFunc: func(context.Context, string) error { return tt.deleteErr }}
			uc, _ := NewStorageCleanupUsecase(jobs, storage, &videoRepositoryMock{}, validStorageCleanupConfig())
			processed, err := uc.ProcessNext(context.Background())
			if err != nil || !processed {
				t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
			}
			if !marked {
				t.Fatal("MarkFailed was not called")
			}
		})
	}
}

func TestStorageCleanupUsecaseRecoverTimedOutJobsBuildsRecoveryInput(t *testing.T) {
	nowJST := time.Date(2026, 8, 3, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	config := validStorageCleanupConfig()
	jobs := &storageCleanupJobRepositoryMock{recoverTimedOutFunc: func(_ context.Context, input repository.StorageCleanupRecoveryInput) (int, error) {
		if !input.Now.Equal(nowJST.UTC()) || !input.TimeoutBefore.Equal(nowJST.UTC().Add(-config.Timeout)) || input.Limit != 25 || input.RetryDelays != config.RetryDelays {
			t.Fatalf("recovery input = %+v", input)
		}
		return 4, nil
	}}
	uc, _ := NewStorageCleanupUsecase(jobs, &objectStorageRepositoryMock{}, &videoRepositoryMock{}, config)
	count, err := uc.RecoverTimedOutJobs(context.Background(), nowJST, 25)
	if err != nil || count != 4 {
		t.Fatalf("RecoverTimedOutJobs() = (%d, %v)", count, err)
	}
	if _, err := uc.RecoverTimedOutJobs(context.Background(), time.Time{}, 1); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("zero now error = %v", err)
	}
	if _, err := uc.RecoverTimedOutJobs(context.Background(), nowJST, 0); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("zero limit error = %v", err)
	}
}

func TestStorageCleanupUsecaseDetectOrphanObjectsRegistersOnlyOldUnreferencedObjectsAcrossPages(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Minute)
	nextCursor := "page-2"
	listCalls := 0
	deleteCalled := false
	storage := &objectStorageRepositoryMock{
		listManagedObjectsFunc: func(_ context.Context, cursor *string, limit int32) (repository.ManagedObjectPage, error) {
			listCalls++
			if limit != 100 {
				t.Fatalf("limit = %d", limit)
			}
			switch listCalls {
			case 1:
				if cursor != nil {
					t.Fatalf("first cursor = %v", cursor)
				}
				return repository.ManagedObjectPage{Items: []repository.ManagedObject{
					{ObjectKey: "", LastModifiedAt: old},
					{ObjectKey: "videos/zero-time.mp4"},
					{ObjectKey: "videos/recent.mp4", LastModifiedAt: recent},
					{ObjectKey: "videos/referenced.mp4", LastModifiedAt: old},
					{ObjectKey: "videos/orphan-1.mp4", LastModifiedAt: old},
				}, HasMore: true, NextCursor: &nextCursor}, nil
			case 2:
				if cursor == nil || *cursor != nextCursor {
					t.Fatalf("second cursor = %v", cursor)
				}
				return repository.ManagedObjectPage{Items: []repository.ManagedObject{{ObjectKey: "videos/orphan-2.jpg", LastModifiedAt: old}}, HasMore: false}, nil
			default:
				t.Fatalf("unexpected list call %d", listCalls)
				return repository.ManagedObjectPage{}, nil
			}
		},
		deleteFunc: func(context.Context, string) error { deleteCalled = true; return nil },
	}
	videos := &videoRepositoryMock{isObjectReferencedFunc: func(_ context.Context, key string) (bool, error) {
		return key == "videos/referenced.mp4", nil
	}}
	var created []*entity.StorageCleanupJob
	jobs := &storageCleanupJobRepositoryMock{createFunc: func(_ context.Context, job *entity.StorageCleanupJob) error {
		created = append(created, job)
		return nil
	}}
	uc, _ := NewStorageCleanupUsecase(jobs, storage, videos, validStorageCleanupConfig())
	count, err := uc.DetectOrphanObjects(context.Background(), now)
	if err != nil || count != 2 {
		t.Fatalf("DetectOrphanObjects() = (%d, %v)", count, err)
	}
	if deleteCalled {
		t.Fatal("orphan detection deleted storage object directly")
	}
	if len(created) != 2 {
		t.Fatalf("created cleanup jobs = %d", len(created))
	}
	for _, job := range created {
		if job.VideoID != nil || job.AssetType != entity.StorageAssetUnknown || job.Cause != entity.StorageCleanupOrphanDetected || job.Status != entity.StorageCleanupQueued {
			t.Fatalf("cleanup job = %+v", job)
		}
	}
}

func TestStorageCleanupUsecaseDetectOrphanObjectsRejectsBrokenPagination(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		list func(context.Context, *string, int32) (repository.ManagedObjectPage, error)
		want string
	}{
		{name: "missing next cursor", list: func(context.Context, *string, int32) (repository.ManagedObjectPage, error) {
			return repository.ManagedObjectPage{HasMore: true}, nil
		}, want: "no next cursor"},
		{name: "cursor does not advance", list: func() func(context.Context, *string, int32) (repository.ManagedObjectPage, error) {
			calls := 0
			cursor := "same"
			return func(_ context.Context, current *string, _ int32) (repository.ManagedObjectPage, error) {
				calls++
				if calls == 1 {
					return repository.ManagedObjectPage{HasMore: true, NextCursor: &cursor}, nil
				}
				return repository.ManagedObjectPage{HasMore: true, NextCursor: &cursor}, nil
			}
		}(), want: "did not advance"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &objectStorageRepositoryMock{listManagedObjectsFunc: tt.list}
			uc, _ := NewStorageCleanupUsecase(&storageCleanupJobRepositoryMock{}, storage, &videoRepositoryMock{}, validStorageCleanupConfig())
			_, err := uc.DetectOrphanObjects(context.Background(), now)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestStorageCleanupUsecaseDetectOrphanObjectsReturnsPartialCountOnLaterError(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	next := "next"
	calls := 0
	storageErr := errors.New("list failed")
	storage := &objectStorageRepositoryMock{listManagedObjectsFunc: func(context.Context, *string, int32) (repository.ManagedObjectPage, error) {
		calls++
		if calls == 1 {
			return repository.ManagedObjectPage{Items: []repository.ManagedObject{{ObjectKey: "videos/orphan.mp4", LastModifiedAt: old}}, HasMore: true, NextCursor: &next}, nil
		}
		return repository.ManagedObjectPage{}, storageErr
	}}
	videos := &videoRepositoryMock{isObjectReferencedFunc: func(context.Context, string) (bool, error) { return false, nil }}
	jobs := &storageCleanupJobRepositoryMock{createFunc: func(context.Context, *entity.StorageCleanupJob) error { return nil }}
	uc, _ := NewStorageCleanupUsecase(jobs, storage, videos, validStorageCleanupConfig())
	count, err := uc.DetectOrphanObjects(context.Background(), now)
	if count != 1 || !errors.Is(err, storageErr) {
		t.Fatalf("DetectOrphanObjects() = (%d, %v)", count, err)
	}
}

func TestStorageCleanupErrorClassification(t *testing.T) {
	code, message, retryable := classifyCleanupError(context.DeadlineExceeded)
	if code != storageCleanupTemporaryCode || message != storageCleanupTimeoutMessage || !retryable {
		t.Fatalf("timeout classification = %q %q %v", code, message, retryable)
	}
	code, message, retryable = classifyCleanupError(entity.ErrStorageUnavailable)
	if code != storageCleanupTemporaryCode || message != storageCleanupTemporaryMessage || !retryable {
		t.Fatalf("temporary classification = %q %q %v", code, message, retryable)
	}
	code, message, retryable = classifyCleanupError(errors.New("access denied"))
	if code != storageCleanupPermanentCode || message != storageCleanupPermanentMessage || retryable {
		t.Fatalf("permanent classification = %q %q %v", code, message, retryable)
	}
}
