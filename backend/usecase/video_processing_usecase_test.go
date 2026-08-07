package usecase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

func validVideoProcessingConfig() VideoProcessingUsecaseConfig {
	return VideoProcessingUsecaseConfig{RetryDelays: [3]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}, ManagedPrefix: "videos/"}
}

func processingClaim(attempt int, sourceMeta *entity.SourceVideoMeta) *repository.ProcessingClaim {
	now := time.Now()
	started := now.Add(-time.Second)
	return &repository.ProcessingClaim{
		Job:        &entity.VideoProcessingJob{ID: 31, VideoID: 22, Status: entity.VideoJobRunning, AttemptCount: attempt, MaxAttempts: 4, StartedAt: &started, AvailableAt: now, CreatedAt: now, UpdatedAt: now},
		Video:      &entity.Video{ID: 22, UserID: 8, OriginalObjectKey: "videos/22/source/source.mp4", ProcessingStatus: entity.VideoProcessingProcessing, PublishStatus: entity.VideoPublishPrivate, CreatedAt: now, UpdatedAt: now},
		SourceMeta: sourceMeta,
	}
}

func validSourceMeta(hasAudio bool) entity.SourceVideoMeta {
	meta := entity.SourceVideoMeta{MIMEType: "video/mp4", Container: "mp4", SizeBytes: 1000, DurationMillis: 9000, Width: 1080, Height: 1920, FrameRate: 30, VideoCodec: "h264", HasAudio: hasAudio}
	if hasAudio {
		meta.AudioCodec = "aac"
	}
	return meta
}

func validOutputMeta(hasAudio bool) entity.OutputVideoMeta {
	meta := entity.OutputVideoMeta{Container: "mp4", Width: 720, Height: 1280, FrameRate: 30, VideoCodec: "h264", HasAudio: hasAudio}
	if hasAudio {
		meta.AudioCodec = "aac"
	}
	return meta
}

func TestNewVideoProcessingUsecaseRejectsInvalidConfiguration(t *testing.T) {
	validVideos := &videoRepositoryMock{}
	validJobs := &videoProcessingJobRepositoryMock{}
	validStorage := &objectStorageRepositoryMock{}
	validMedia := &mediaRepositoryMock{}
	validCleanup := &storageCleanupJobRepositoryMock{}

	tests := []struct {
		name    string
		videos  repository.IVideoRepository
		jobs    repository.IVideoProcessingJobRepository
		storage repository.IObjectStorageRepository
		media   repository.IMediaRepository
		cleanup repository.IStorageCleanupJobRepository
		mutate  func(*VideoProcessingUsecaseConfig)
	}{
		{name: "videos missing", jobs: validJobs, storage: validStorage, media: validMedia, cleanup: validCleanup},
		{name: "jobs missing", videos: validVideos, storage: validStorage, media: validMedia, cleanup: validCleanup},
		{name: "storage missing", videos: validVideos, jobs: validJobs, media: validMedia, cleanup: validCleanup},
		{name: "media missing", videos: validVideos, jobs: validJobs, storage: validStorage, cleanup: validCleanup},
		{name: "cleanup missing", videos: validVideos, jobs: validJobs, storage: validStorage, media: validMedia},
		{name: "invalid prefix", videos: validVideos, jobs: validJobs, storage: validStorage, media: validMedia, cleanup: validCleanup, mutate: func(c *VideoProcessingUsecaseConfig) { c.ManagedPrefix = "other/" }},
		{name: "zero retry delay", videos: validVideos, jobs: validJobs, storage: validStorage, media: validMedia, cleanup: validCleanup, mutate: func(c *VideoProcessingUsecaseConfig) { c.RetryDelays[1] = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validVideoProcessingConfig()
			if tt.mutate != nil {
				tt.mutate(&config)
			}
			if _, err := NewVideoProcessingUsecase(tt.videos, tt.jobs, tt.storage, tt.media, tt.cleanup, config); err == nil {
				t.Fatal("NewVideoProcessingUsecase() error = nil")
			}
		})
	}
}

func TestVideoProcessingUsecaseProcessNextReturnsNoWorkWhenClaimIsNil(t *testing.T) {
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return nil, nil }}
	uc, _ := NewVideoProcessingUsecase(&videoRepositoryMock{}, jobs, &objectStorageRepositoryMock{}, &mediaRepositoryMock{}, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	processed, err := uc.ProcessNext(context.Background())
	if err != nil || processed {
		t.Fatalf("ProcessNext() = (%v, %v), want (false, nil)", processed, err)
	}
}

func TestVideoProcessingUsecaseProcessNextRejectsInconsistentClaim(t *testing.T) {
	claim := processingClaim(1, nil)
	claim.Job.VideoID = claim.Video.ID + 1
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil }}
	uc, _ := NewVideoProcessingUsecase(&videoRepositoryMock{}, jobs, &objectStorageRepositoryMock{}, &mediaRepositoryMock{}, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	processed, err := uc.ProcessNext(context.Background())
	if !processed || !errors.Is(err, entity.ErrProcessingJobConflict) {
		t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
	}
}

func TestVideoProcessingUsecaseProcessNextRunsFullPipelineAndRemovesWorkDirectory(t *testing.T) {
	claim := processingClaim(1, nil)
	var workDir string
	var recordCalled, completeCalled bool
	videos := &videoRepositoryMock{
		recordSourceValidationFunc: func(_ context.Context, videoID uint64, meta entity.SourceVideoMeta, now time.Time) error {
			recordCalled = true
			if videoID != claim.Video.ID || !meta.HasAudio {
				t.Fatalf("RecordSourceValidation(%d, %+v, %s)", videoID, meta, now)
			}
			return nil
		},
		completeProcessingFunc: func(_ context.Context, input repository.ProcessingCompletionInput) error {
			completeCalled = true
			if input.JobID != claim.Job.ID {
				t.Fatalf("completion input = %+v", input)
			}
			if !regexp.MustCompile(`^videos/22/output/[0-9a-f]{32}\.mp4$`).MatchString(input.OutputMeta.VideoObjectKey) {
				t.Fatalf("video key = %q", input.OutputMeta.VideoObjectKey)
			}
			if !regexp.MustCompile(`^videos/22/thumbnail/[0-9a-f]{32}\.jpg$`).MatchString(input.OutputMeta.ThumbnailObjectKey) {
				t.Fatalf("thumbnail key = %q", input.OutputMeta.ThumbnailObjectKey)
			}
			return nil
		},
	}
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil }}
	storage := &objectStorageRepositoryMock{
		downloadFunc: func(_ context.Context, key, destination string) error {
			if key != claim.Video.OriginalObjectKey {
				t.Fatalf("download key = %q", key)
			}
			workDir = filepath.Dir(destination)
			return os.WriteFile(destination, []byte("source"), 0o600)
		},
		uploadProcessedFunc: func(_ context.Context, key, source string) error {
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("processed source missing: %v", err)
			}
			return nil
		},
		uploadThumbnailFunc: func(_ context.Context, key, source string) error {
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("thumbnail source missing: %v", err)
			}
			return nil
		},
	}
	media := &mediaRepositoryMock{
		probeFunc: func(_ context.Context, path string) (entity.SourceVideoMeta, error) {
			if filepath.Base(path) != "source.bin" {
				t.Fatalf("probe path = %q", path)
			}
			return validSourceMeta(true), nil
		},
		transcodeFunc: func(_ context.Context, input, output string, hasAudio bool) error {
			if !hasAudio || filepath.Base(input) != "source.bin" || filepath.Base(output) != "output.mp4" {
				t.Fatalf("Transcode(%q,%q,%v)", input, output, hasAudio)
			}
			return os.WriteFile(output, []byte("output"), 0o600)
		},
		generateThumbnailFunc: func(_ context.Context, input, output string) error {
			return os.WriteFile(output, []byte("thumbnail"), 0o600)
		},
		probeOutputFunc: func(_ context.Context, path string) (entity.OutputVideoMeta, error) {
			if filepath.Base(path) != "output.mp4" {
				t.Fatalf("output probe path = %q", path)
			}
			return validOutputMeta(true), nil
		},
	}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, storage, media, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())

	processed, err := uc.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessNext() = (%v, %v)", processed, err)
	}
	if !recordCalled || !completeCalled {
		t.Fatalf("recordCalled=%v completeCalled=%v", recordCalled, completeCalled)
	}
	if workDir == "" {
		t.Fatal("work directory was not captured")
	}
	if _, err := os.Stat(workDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("work directory still exists: %s, err=%v", workDir, err)
	}
}

func TestVideoProcessingUsecaseSkipsProbeWhenSourceMetaAlreadyExists(t *testing.T) {
	meta := validSourceMeta(false)
	claim := processingClaim(2, &meta)
	probeCalled := false
	videos := &videoRepositoryMock{completeProcessingFunc: func(context.Context, repository.ProcessingCompletionInput) error { return nil }}
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil }}
	storage := &objectStorageRepositoryMock{
		downloadFunc:        func(_ context.Context, _, path string) error { return os.WriteFile(path, []byte("source"), 0o600) },
		uploadProcessedFunc: func(context.Context, string, string) error { return nil },
		uploadThumbnailFunc: func(context.Context, string, string) error { return nil },
	}
	media := &mediaRepositoryMock{
		probeFunc: func(context.Context, string) (entity.SourceVideoMeta, error) {
			probeCalled = true
			return entity.SourceVideoMeta{}, nil
		},
		transcodeFunc: func(_ context.Context, _, output string, hasAudio bool) error {
			if hasAudio {
				t.Fatal("hasAudio=true, want false")
			}
			return os.WriteFile(output, []byte("output"), 0o600)
		},
		generateThumbnailFunc: func(_ context.Context, _, output string) error { return os.WriteFile(output, []byte("thumb"), 0o600) },
		probeOutputFunc:       func(context.Context, string) (entity.OutputVideoMeta, error) { return validOutputMeta(false), nil },
	}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, storage, media, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	if processed, err := uc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if probeCalled {
		t.Fatal("Probe was called despite existing SourceMeta")
	}
}

func TestVideoProcessingUsecaseCancelsJobWhenVideoDisappearsAfterSourceValidation(t *testing.T) {
	claim := processingClaim(1, nil)
	cancelCalled := false
	videos := &videoRepositoryMock{recordSourceValidationFunc: func(context.Context, uint64, entity.SourceVideoMeta, time.Time) error { return entity.ErrVideoNotFound }}
	jobs := &videoProcessingJobRepositoryMock{
		claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil },
		cancelFunc: func(_ context.Context, jobID uint64, now time.Time) error {
			cancelCalled = true
			if jobID != claim.Job.ID {
				t.Fatalf("Cancel(%d,%s)", jobID, now)
			}
			return nil
		},
	}
	storage := &objectStorageRepositoryMock{downloadFunc: func(_ context.Context, _, path string) error { return os.WriteFile(path, []byte("source"), 0o600) }}
	media := &mediaRepositoryMock{probeFunc: func(context.Context, string) (entity.SourceVideoMeta, error) { return validSourceMeta(false), nil }}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, storage, media, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	if processed, err := uc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if !cancelCalled {
		t.Fatal("Cancel was not called")
	}
}

func TestVideoProcessingUsecasePermanentMediaFailureDoesNotRetry(t *testing.T) {
	claim := processingClaim(1, nil)
	failCalled := false
	videos := &videoRepositoryMock{failProcessingFunc: func(_ context.Context, input repository.ProcessingFailureInput) error {
		failCalled = true
		if input.FailureCode != entity.VideoFailureInvalidFormat || input.FailureMessage != "unsupported real format" {
			t.Fatalf("failure input = %+v", input)
		}
		return nil
	}}
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil }}
	storage := &objectStorageRepositoryMock{downloadFunc: func(_ context.Context, _, path string) error { return os.WriteFile(path, []byte("bad"), 0o600) }}
	media := &mediaRepositoryMock{probeFunc: func(context.Context, string) (entity.SourceVideoMeta, error) {
		return entity.SourceVideoMeta{}, &repository.MediaError{Code: entity.VideoFailureInvalidFormat, Retryable: false, Message: "unsupported real format"}
	}}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, storage, media, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	if processed, err := uc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if !failCalled {
		t.Fatal("FailProcessing was not called")
	}
}

func TestVideoProcessingUsecaseTemporaryStorageFailureSchedulesRetry(t *testing.T) {
	claim := processingClaim(1, nil)
	var availableAt, scheduledAt time.Time
	jobs := &videoProcessingJobRepositoryMock{
		claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil },
		scheduleRetryFunc: func(_ context.Context, jobID uint64, gotAvailable time.Time, code, message string, now time.Time) error {
			availableAt, scheduledAt = gotAvailable, now
			if jobID != claim.Job.ID || code != string(entity.VideoFailureStorageUnavailable) || message != "object storage is temporarily unavailable" {
				t.Fatalf("retry args = %d %q %q", jobID, code, message)
			}
			return nil
		},
	}
	storage := &objectStorageRepositoryMock{downloadFunc: func(context.Context, string, string) error { return entity.ErrStorageUnavailable }}
	uc, _ := NewVideoProcessingUsecase(&videoRepositoryMock{}, jobs, storage, &mediaRepositoryMock{}, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	if processed, err := uc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if availableAt.Sub(scheduledAt) != time.Minute {
		t.Fatalf("retry delay = %s, want 1m", availableAt.Sub(scheduledAt))
	}
}

func TestVideoProcessingUsecaseFourthAttemptBecomesFinalFailure(t *testing.T) {
	claim := processingClaim(4, nil)
	failCalled := false
	videos := &videoRepositoryMock{failProcessingFunc: func(_ context.Context, input repository.ProcessingFailureInput) error {
		failCalled = true
		if input.FailureCode != entity.VideoFailureStorageUnavailable {
			t.Fatalf("code=%s", input.FailureCode)
		}
		return nil
	}}
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil }}
	storage := &objectStorageRepositoryMock{downloadFunc: func(context.Context, string, string) error { return entity.ErrStorageUnavailable }}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, storage, &mediaRepositoryMock{}, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	if processed, err := uc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if !failCalled {
		t.Fatal("FailProcessing was not called on fourth attempt")
	}
}

func TestVideoProcessingUsecaseThumbnailUploadFailureRegistersProcessedObjectCleanup(t *testing.T) {
	meta := validSourceMeta(false)
	claim := processingClaim(1, &meta)
	var cleanupJobs []*entity.StorageCleanupJob
	jobs := &videoProcessingJobRepositoryMock{
		claimNextFunc:     func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil },
		scheduleRetryFunc: func(context.Context, uint64, time.Time, string, string, time.Time) error { return nil },
	}
	storage := &objectStorageRepositoryMock{
		downloadFunc:        func(_ context.Context, _, path string) error { return os.WriteFile(path, []byte("source"), 0o600) },
		uploadProcessedFunc: func(context.Context, string, string) error { return nil },
		uploadThumbnailFunc: func(context.Context, string, string) error { return entity.ErrStorageUnavailable },
	}
	media := &mediaRepositoryMock{
		transcodeFunc: func(_ context.Context, _, output string, _ bool) error {
			return os.WriteFile(output, []byte("out"), 0o600)
		},
		generateThumbnailFunc: func(_ context.Context, _, output string) error { return os.WriteFile(output, []byte("thumb"), 0o600) },
		probeOutputFunc:       func(context.Context, string) (entity.OutputVideoMeta, error) { return validOutputMeta(false), nil },
	}
	cleanup := &storageCleanupJobRepositoryMock{createFunc: func(_ context.Context, job *entity.StorageCleanupJob) error {
		cleanupJobs = append(cleanupJobs, job)
		return nil
	}}
	uc, _ := NewVideoProcessingUsecase(&videoRepositoryMock{}, jobs, storage, media, cleanup, validVideoProcessingConfig())
	if processed, err := uc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if len(cleanupJobs) != 1 || cleanupJobs[0].AssetType != entity.StorageAssetProcessed || cleanupJobs[0].Cause != entity.StorageCleanupRollback {
		t.Fatalf("cleanupJobs=%+v", cleanupJobs)
	}
	if cleanupJobs[0].VideoID == nil || *cleanupJobs[0].VideoID != claim.Video.ID {
		t.Fatalf("cleanup video id = %v", cleanupJobs[0].VideoID)
	}
}

func TestVideoProcessingUsecaseCompletionFailureRegistersBothUploadedObjectsForCleanup(t *testing.T) {
	meta := validSourceMeta(false)
	claim := processingClaim(1, &meta)
	completeErr := errors.New("db completion failed")
	var cleanupJobs []*entity.StorageCleanupJob
	videos := &videoRepositoryMock{completeProcessingFunc: func(context.Context, repository.ProcessingCompletionInput) error { return completeErr }}
	jobs := &videoProcessingJobRepositoryMock{claimNextFunc: func(context.Context, time.Time) (*repository.ProcessingClaim, error) { return claim, nil }}
	storage := &objectStorageRepositoryMock{
		downloadFunc:        func(_ context.Context, _, path string) error { return os.WriteFile(path, []byte("source"), 0o600) },
		uploadProcessedFunc: func(context.Context, string, string) error { return nil },
		uploadThumbnailFunc: func(context.Context, string, string) error { return nil },
	}
	media := &mediaRepositoryMock{
		transcodeFunc: func(_ context.Context, _, output string, _ bool) error {
			return os.WriteFile(output, []byte("out"), 0o600)
		},
		generateThumbnailFunc: func(_ context.Context, _, output string) error { return os.WriteFile(output, []byte("thumb"), 0o600) },
		probeOutputFunc:       func(context.Context, string) (entity.OutputVideoMeta, error) { return validOutputMeta(false), nil },
	}
	cleanup := &storageCleanupJobRepositoryMock{createFunc: func(_ context.Context, job *entity.StorageCleanupJob) error {
		cleanupJobs = append(cleanupJobs, job)
		return nil
	}}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, storage, media, cleanup, validVideoProcessingConfig())
	processed, err := uc.ProcessNext(context.Background())
	if !processed || !errors.Is(err, completeErr) {
		t.Fatalf("ProcessNext()=(%v,%v)", processed, err)
	}
	if len(cleanupJobs) != 2 {
		t.Fatalf("cleanup job count = %d, want 2", len(cleanupJobs))
	}
}

func TestVideoProcessingUsecaseMaintenanceInputsAndForwarding(t *testing.T) {
	nowJST := time.Date(2026, 8, 3, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	videos := &videoRepositoryMock{
		expireUploadsFunc: func(_ context.Context, now time.Time, limit int) (int, error) {
			if limit != 10 {
				t.Fatalf("ExpireUploads(%s,%d)", now, limit)
			}
			return 3, nil
		},
		deleteExpiredIdempotencyRecordsFunc: func(_ context.Context, now time.Time, limit int) (int64, error) {
			if limit != 20 {
				t.Fatalf("DeleteExpired(%s,%d)", now, limit)
			}
			return 4, nil
		},
	}
	jobs := &videoProcessingJobRepositoryMock{recoverTimedOutFunc: func(_ context.Context, input repository.ProcessingRecoveryInput) (int, error) {
		if !input.Now.Equal(nowJST) || !input.TimeoutBefore.Equal(nowJST.Add(-videoProcessingTimeout)) || input.Limit != 5 || input.RetryDelays != validVideoProcessingConfig().RetryDelays {
			t.Fatalf("recovery input=%+v", input)
		}
		return 2, nil
	}}
	uc, _ := NewVideoProcessingUsecase(videos, jobs, &objectStorageRepositoryMock{}, &mediaRepositoryMock{}, &storageCleanupJobRepositoryMock{}, validVideoProcessingConfig())
	if count, err := uc.ExpireUploads(context.Background(), nowJST, 10); err != nil || count != 3 {
		t.Fatalf("ExpireUploads=(%d,%v)", count, err)
	}
	if count, err := uc.RecoverTimedOutJobs(context.Background(), nowJST, 5); err != nil || count != 2 {
		t.Fatalf("Recover=(%d,%v)", count, err)
	}
	if count, err := uc.DeleteExpiredIdempotencyRecords(context.Background(), nowJST, 20); err != nil || count != 4 {
		t.Fatalf("DeleteExpired=(%d,%v)", count, err)
	}
	if _, err := uc.ExpireUploads(context.Background(), time.Time{}, 1); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("zero now error=%v", err)
	}
	if _, err := uc.RecoverTimedOutJobs(context.Background(), nowJST, 0); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("zero limit error=%v", err)
	}
}

func TestBuildProcessingObjectKeyEnforcesAssetExtensionPair(t *testing.T) {
	valid := []struct{ kind, ext, pattern string }{
		{"output", "mp4", `^videos/5/output/[0-9a-f]{32}\.mp4$`},
		{"thumbnail", "jpg", `^videos/5/thumbnail/[0-9a-f]{32}\.jpg$`},
	}
	for _, tt := range valid {
		key, err := buildProcessingObjectKey("videos/", 5, tt.kind, tt.ext)
		if err != nil || !regexp.MustCompile(tt.pattern).MatchString(key) {
			t.Fatalf("buildProcessingObjectKey()=(%q,%v)", key, err)
		}
	}
	invalid := []struct {
		prefix    string
		id        uint64
		kind, ext string
	}{{"other/", 5, "output", "mp4"}, {"videos/", 0, "output", "mp4"}, {"videos/", 5, "source", "mp4"}, {"videos/", 5, "output", "jpg"}, {"videos/", 5, "thumbnail", "mp4"}}
	for _, tt := range invalid {
		if _, err := buildProcessingObjectKey(tt.prefix, tt.id, tt.kind, tt.ext); !errors.Is(err, entity.ErrInvalidInput) {
			t.Fatalf("invalid args %+v error=%v", tt, err)
		}
	}
}

func TestProcessingFailureHelpersUseSafeMessages(t *testing.T) {
	failure := classifyProcessingError(&repository.MediaError{Code: entity.VideoFailureDurationExceeded, Retryable: false, Message: "duration exceeded"})
	if failure.Code != entity.VideoFailureDurationExceeded || failure.Retryable || failure.Message != "duration exceeded" {
		t.Fatalf("failure=%+v", failure)
	}
	codeMessage := classifyProcessingError(entity.ErrObjectNotFound)
	if codeMessage.Retryable || !strings.Contains(codeMessage.Message, "not found") {
		t.Fatalf("object not found=%+v", codeMessage)
	}
	if got := normalizeProcessingFailureMessage("   "); got != "video processing failed" {
		t.Fatalf("empty message=%q", got)
	}
	long := strings.Repeat("あ", 501)
	if got := normalizeProcessingFailureMessage(long); len([]rune(got)) != 500 {
		t.Fatalf("message rune length=%d", len([]rune(got)))
	}
}
