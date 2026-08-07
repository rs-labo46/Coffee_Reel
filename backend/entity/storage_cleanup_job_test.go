package entity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewStorageCleanupJob(t *testing.T) {
	now := testCleanupTime()
	videoID := uint64(1)
	job, err := NewStorageCleanupJob(&videoID, "  videos/1/source/a.mp4  ", StorageAssetOriginal, StorageCleanupVideoDelete, now)
	if err != nil {
		t.Fatal(err)
	}
	if job.ObjectKey != "videos/1/source/a.mp4" || job.Status != StorageCleanupQueued || job.AttemptCount != 0 || job.MaxAttempts != 4 {
		t.Fatalf("unexpected cleanup job: %#v", job)
	}
	if !job.AvailableAt.Equal(now) || job.StartedAt != nil || job.FinishedAt != nil {
		t.Fatalf("unexpected initial timestamps: %#v", job)
	}

	orphan, err := NewStorageCleanupJob(nil, "videos/orphan.mp4", StorageAssetUnknown, StorageCleanupOrphanDetected, now)
	if err != nil {
		t.Fatal(err)
	}
	if orphan.VideoID != nil {
		t.Fatalf("orphan cleanup must not require a video: %#v", orphan)
	}
}

func TestNewStorageCleanupJobRejectsInvalidInput(t *testing.T) {
	now := testCleanupTime()
	validVideoID := uint64(1)
	zeroVideoID := uint64(0)

	tests := []struct {
		name      string
		videoID   *uint64
		objectKey string
		assetType StorageAssetType
		cause     StorageCleanupCause
		now       time.Time
	}{
		{name: "zero video id", videoID: &zeroVideoID, objectKey: "videos/1/a.mp4", assetType: StorageAssetOriginal, cause: StorageCleanupVideoDelete, now: now},
		{name: "empty object key", videoID: &validVideoID, objectKey: "   ", assetType: StorageAssetOriginal, cause: StorageCleanupVideoDelete, now: now},
		{name: "invalid asset type", videoID: &validVideoID, objectKey: "videos/1/a.mp4", assetType: StorageAssetType("invalid"), cause: StorageCleanupVideoDelete, now: now},
		{name: "invalid cause", videoID: &validVideoID, objectKey: "videos/1/a.mp4", assetType: StorageAssetOriginal, cause: StorageCleanupCause("invalid"), now: now},
		{name: "zero time", videoID: &validVideoID, objectKey: "videos/1/a.mp4", assetType: StorageAssetOriginal, cause: StorageCleanupVideoDelete, now: time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewStorageCleanupJob(tt.videoID, tt.objectKey, tt.assetType, tt.cause, tt.now); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestStorageCleanupJobClaimAndRetry(t *testing.T) {
	now := testCleanupTime()
	job := mustNewCleanupJob(t, now)

	if err := job.Claim(now); err != nil {
		t.Fatal(err)
	}
	if job.Status != StorageCleanupRunning || job.AttemptCount != 1 || job.StartedAt == nil {
		t.Fatalf("unexpected claimed cleanup job: %#v", job)
	}

	availableAt := now.Add(time.Minute)
	if err := job.ScheduleRetry(availableAt, "storage_delete_failed", "  storage deletion failed  ", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.Status != StorageCleanupRetryWait || !job.AvailableAt.Equal(availableAt) {
		t.Fatalf("unexpected retry job: %#v", job)
	}
	if job.LastErrorCode != "storage_delete_failed" || job.LastErrorMessage != "storage deletion failed" {
		t.Fatalf("unexpected retry details: %#v", job)
	}
}

func TestStorageCleanupJobClaimRejectsInvalidState(t *testing.T) {
	now := testCleanupTime()

	tests := []struct {
		name string
		job  *StorageCleanupJob
		at   time.Time
		err  error
	}{
		{name: "zero time", job: mustNewCleanupJob(t, now), at: time.Time{}, err: ErrInvalidInput},
		{name: "before available", job: mustNewCleanupJob(t, now), at: now.Add(-time.Second), err: ErrCleanupJobConflict},
		{name: "max attempts reached", job: &StorageCleanupJob{Status: StorageCleanupQueued, AttemptCount: 4, MaxAttempts: 4, AvailableAt: now}, at: now, err: ErrCleanupJobConflict},
		{name: "negative attempts", job: &StorageCleanupJob{Status: StorageCleanupQueued, AttemptCount: -1, MaxAttempts: 4, AvailableAt: now}, at: now, err: ErrCleanupJobConflict},
		{name: "invalid max attempts", job: &StorageCleanupJob{Status: StorageCleanupQueued, AttemptCount: 0, MaxAttempts: 3, AvailableAt: now}, at: now, err: ErrCleanupJobConflict},
		{name: "terminal state", job: &StorageCleanupJob{Status: StorageCleanupSucceeded, AttemptCount: 1, MaxAttempts: 4, AvailableAt: now}, at: now, err: ErrCleanupJobConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.job.Claim(tt.at); !errors.Is(err, tt.err) {
				t.Fatalf("expected %v, got %v", tt.err, err)
			}
		})
	}
}

func TestStorageCleanupJobScheduleRetryRejectsInvalidInput(t *testing.T) {
	now := testCleanupTime()
	newRunningJob := func() *StorageCleanupJob {
		job := mustNewCleanupJob(t, now)
		if err := job.Claim(now); err != nil {
			t.Fatal(err)
		}
		return job
	}

	tests := []struct {
		name        string
		job         *StorageCleanupJob
		availableAt time.Time
		code        string
		message     string
		now         time.Time
		err         error
	}{
		{name: "zero now", job: newRunningJob(), availableAt: now.Add(time.Minute), code: "storage_delete_failed", message: "failed", now: time.Time{}, err: ErrInvalidInput},
		{name: "zero available", job: newRunningJob(), availableAt: time.Time{}, code: "storage_delete_failed", message: "failed", now: now, err: ErrInvalidInput},
		{name: "invalid uppercase code", job: newRunningJob(), availableAt: now.Add(time.Minute), code: "STORAGE_FAILED", message: "failed", now: now, err: ErrInvalidInput},
		{name: "invalid hyphen code", job: newRunningJob(), availableAt: now.Add(time.Minute), code: "storage-failed", message: "failed", now: now, err: ErrInvalidInput},
		{name: "code too long", job: newRunningJob(), availableAt: now.Add(time.Minute), code: strings.Repeat("a", 65), message: "failed", now: now, err: ErrInvalidInput},
		{name: "empty message", job: newRunningJob(), availableAt: now.Add(time.Minute), code: "storage_delete_failed", message: " ", now: now, err: ErrInvalidInput},
		{name: "message too long", job: newRunningJob(), availableAt: now.Add(time.Minute), code: "storage_delete_failed", message: strings.Repeat("a", 501), now: now, err: ErrInvalidInput},
		{name: "available not after now", job: newRunningJob(), availableAt: now, code: "storage_delete_failed", message: "failed", now: now, err: ErrCleanupJobConflict},
		{name: "not running", job: mustNewCleanupJob(t, now), availableAt: now.Add(time.Minute), code: "storage_delete_failed", message: "failed", now: now, err: ErrCleanupJobConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.job.ScheduleRetry(tt.availableAt, tt.code, tt.message, tt.now); !errors.Is(err, tt.err) {
				t.Fatalf("expected %v, got %v", tt.err, err)
			}
		})
	}
}

func TestStorageCleanupJobMarkSucceeded(t *testing.T) {
	now := testCleanupTime()
	job := mustNewCleanupJob(t, now)
	if err := job.Claim(now); err != nil {
		t.Fatal(err)
	}
	job.LastErrorCode = "worker_timeout"
	job.LastErrorMessage = "worker timed out"

	if err := job.MarkSucceeded(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.Status != StorageCleanupSucceeded || job.FinishedAt == nil {
		t.Fatalf("unexpected succeeded cleanup job: %#v", job)
	}
	if job.LastErrorCode != "" || job.LastErrorMessage != "" {
		t.Fatalf("success must clear previous error: %#v", job)
	}
	if err := job.Claim(now.Add(time.Minute)); !errors.Is(err, ErrCleanupJobConflict) {
		t.Fatalf("succeeded job must not be reclaimed: %v", err)
	}
}

func TestStorageCleanupJobMarkFailed(t *testing.T) {
	now := testCleanupTime()
	job := mustNewCleanupJob(t, now)
	if err := job.Claim(now); err != nil {
		t.Fatal(err)
	}

	if err := job.MarkFailed("worker_timeout", "  cleanup worker timed out  ", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.Status != StorageCleanupFailed || job.FinishedAt == nil {
		t.Fatalf("unexpected failed cleanup job: %#v", job)
	}
	if job.LastErrorCode != "worker_timeout" || job.LastErrorMessage != "cleanup worker timed out" {
		t.Fatalf("unexpected failure details: %#v", job)
	}
}

func TestStorageCleanupJobMarkFailedRejectsInvalidDetails(t *testing.T) {
	now := testCleanupTime()
	newRunningJob := func() *StorageCleanupJob {
		job := mustNewCleanupJob(t, now)
		if err := job.Claim(now); err != nil {
			t.Fatal(err)
		}
		return job
	}

	if err := newRunningJob().MarkFailed("INVALID", "failed", now.Add(time.Second)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid code, got %v", err)
	}
	if err := newRunningJob().MarkFailed("worker_timeout", "", now.Add(time.Second)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty message, got %v", err)
	}
	if err := newRunningJob().MarkFailed("worker_timeout", "failed", time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for zero time, got %v", err)
	}
}

func TestStorageCleanupValueValidation(t *testing.T) {
	if !StorageAssetOriginal.IsValid() || StorageAssetType("invalid").IsValid() {
		t.Fatal("unexpected asset type validation result")
	}
	if !StorageCleanupVideoDelete.IsValid() || StorageCleanupCause("invalid").IsValid() {
		t.Fatal("unexpected cleanup cause validation result")
	}
	if !StorageCleanupQueued.IsValid() || StorageCleanupStatus("invalid").IsValid() {
		t.Fatal("unexpected cleanup status validation result")
	}
}

func testCleanupTime() time.Time {
	return time.Date(2026, 7, 28, 4, 0, 0, 0, time.FixedZone("JST", 9*60*60))
}

func mustNewCleanupJob(t *testing.T, now time.Time) *StorageCleanupJob {
	t.Helper()
	videoID := uint64(1)
	job, err := NewStorageCleanupJob(&videoID, "videos/1/source/a.mp4", StorageAssetOriginal, StorageCleanupVideoDelete, now)
	if err != nil {
		t.Fatal(err)
	}
	return job
}
