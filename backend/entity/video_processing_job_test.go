package entity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewVideoProcessingJob(t *testing.T) {
	now := testJobTime()
	job, err := NewVideoProcessingJob(1, now)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != VideoJobQueued || job.AttemptCount != 0 || job.MaxAttempts != 4 {
		t.Fatalf("unexpected initial job: %#v", job)
	}
	if !job.AvailableAt.Equal(now) || job.StartedAt != nil || job.FinishedAt != nil {
		t.Fatalf("unexpected initial timestamps: %#v", job)
	}
}

func TestNewVideoProcessingJobRejectsInvalidInput(t *testing.T) {
	if _, err := NewVideoProcessingJob(0, testJobTime()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for zero video ID, got %v", err)
	}
	if _, err := NewVideoProcessingJob(1, time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for zero time, got %v", err)
	}
}

func TestVideoProcessingJobClaimAndRetry(t *testing.T) {
	now := testJobTime()
	job := mustNewVideoJob(t, now)

	if err := job.Claim(now); err != nil {
		t.Fatal(err)
	}
	if job.Status != VideoJobRunning || job.AttemptCount != 1 || job.StartedAt == nil {
		t.Fatalf("unexpected claimed job: %#v", job)
	}

	availableAt := now.Add(time.Minute)
	if err := job.ScheduleRetry(availableAt, string(VideoFailureStorageUnavailable), "  temporary failure  ", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.Status != VideoJobRetryWait || !job.AvailableAt.Equal(availableAt) {
		t.Fatalf("unexpected retry state: %#v", job)
	}
	if job.LastErrorCode != string(VideoFailureStorageUnavailable) || job.LastErrorMessage != "temporary failure" {
		t.Fatalf("unexpected retry error: %#v", job)
	}
}

func TestVideoProcessingJobClaimRejectsInvalidState(t *testing.T) {
	now := testJobTime()

	tests := []struct {
		name string
		job  *VideoProcessingJob
		at   time.Time
		err  error
	}{
		{name: "zero time", job: mustNewVideoJob(t, now), at: time.Time{}, err: ErrInvalidInput},
		{name: "before available", job: mustNewVideoJob(t, now), at: now.Add(-time.Second), err: ErrProcessingJobConflict},
		{name: "max attempts reached", job: &VideoProcessingJob{Status: VideoJobQueued, AttemptCount: 4, MaxAttempts: 4, AvailableAt: now}, at: now, err: ErrProcessingJobConflict},
		{name: "negative attempts", job: &VideoProcessingJob{Status: VideoJobQueued, AttemptCount: -1, MaxAttempts: 4, AvailableAt: now}, at: now, err: ErrProcessingJobConflict},
		{name: "invalid max attempts", job: &VideoProcessingJob{Status: VideoJobQueued, AttemptCount: 0, MaxAttempts: 3, AvailableAt: now}, at: now, err: ErrProcessingJobConflict},
		{name: "terminal state", job: &VideoProcessingJob{Status: VideoJobSucceeded, AttemptCount: 1, MaxAttempts: 4, AvailableAt: now}, at: now, err: ErrProcessingJobConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.job.Claim(tt.at); !errors.Is(err, tt.err) {
				t.Fatalf("expected %v, got %v", tt.err, err)
			}
		})
	}
}

func TestVideoProcessingJobScheduleRetryRejectsInvalidInput(t *testing.T) {
	now := testJobTime()
	newRunningJob := func() *VideoProcessingJob {
		job := mustNewVideoJob(t, now)
		if err := job.Claim(now); err != nil {
			t.Fatal(err)
		}
		return job
	}

	tests := []struct {
		name        string
		job         *VideoProcessingJob
		availableAt time.Time
		code        string
		message     string
		now         time.Time
		err         error
	}{
		{name: "zero now", job: newRunningJob(), availableAt: now.Add(time.Minute), code: string(VideoFailureStorageUnavailable), message: "failed", now: time.Time{}, err: ErrInvalidInput},
		{name: "zero available", job: newRunningJob(), availableAt: time.Time{}, code: string(VideoFailureStorageUnavailable), message: "failed", now: now, err: ErrInvalidInput},
		{name: "invalid code", job: newRunningJob(), availableAt: now.Add(time.Minute), code: "unknown", message: "failed", now: now, err: ErrInvalidInput},
		{name: "empty message", job: newRunningJob(), availableAt: now.Add(time.Minute), code: string(VideoFailureProcessingFailed), message: "   ", now: now, err: ErrInvalidInput},
		{name: "message too long", job: newRunningJob(), availableAt: now.Add(time.Minute), code: string(VideoFailureProcessingFailed), message: strings.Repeat("a", 501), now: now, err: ErrInvalidInput},
		{name: "available not after now", job: newRunningJob(), availableAt: now, code: string(VideoFailureProcessingFailed), message: "failed", now: now, err: ErrProcessingJobConflict},
		{name: "not running", job: mustNewVideoJob(t, now), availableAt: now.Add(time.Minute), code: string(VideoFailureProcessingFailed), message: "failed", now: now, err: ErrProcessingJobConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.job.ScheduleRetry(tt.availableAt, tt.code, tt.message, tt.now); !errors.Is(err, tt.err) {
				t.Fatalf("expected %v, got %v", tt.err, err)
			}
		})
	}
}

func TestVideoProcessingJobMarkSucceeded(t *testing.T) {
	now := testJobTime()
	job := mustNewVideoJob(t, now)
	if err := job.Claim(now); err != nil {
		t.Fatal(err)
	}
	job.LastErrorCode = string(VideoFailureStorageUnavailable)
	job.LastErrorMessage = "temporary failure"

	if err := job.MarkSucceeded(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.Status != VideoJobSucceeded || job.FinishedAt == nil {
		t.Fatalf("unexpected succeeded job: %#v", job)
	}
	if job.LastErrorCode != "" || job.LastErrorMessage != "" {
		t.Fatalf("success must clear previous error: %#v", job)
	}
	if err := job.Claim(now.Add(time.Minute)); !errors.Is(err, ErrProcessingJobConflict) {
		t.Fatalf("succeeded job must not be reclaimed: %v", err)
	}
}

func TestVideoProcessingJobMarkFailed(t *testing.T) {
	now := testJobTime()
	job := mustNewVideoJob(t, now)
	if err := job.Claim(now); err != nil {
		t.Fatal(err)
	}

	if err := job.MarkFailed(string(VideoFailureInvalidFormat), "  unsupported container  ", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if job.Status != VideoJobFailed || job.FinishedAt == nil {
		t.Fatalf("unexpected failed job: %#v", job)
	}
	if job.LastErrorCode != string(VideoFailureInvalidFormat) || job.LastErrorMessage != "unsupported container" {
		t.Fatalf("unexpected failure details: %#v", job)
	}
}

func TestVideoProcessingJobMarkFailedRejectsInvalidDetails(t *testing.T) {
	now := testJobTime()
	newRunningJob := func() *VideoProcessingJob {
		job := mustNewVideoJob(t, now)
		if err := job.Claim(now); err != nil {
			t.Fatal(err)
		}
		return job
	}

	if err := newRunningJob().MarkFailed("unknown", "failed", now.Add(time.Second)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid code, got %v", err)
	}
	if err := newRunningJob().MarkFailed(string(VideoFailureProcessingFailed), "", now.Add(time.Second)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty message, got %v", err)
	}
	if err := newRunningJob().MarkFailed(string(VideoFailureProcessingFailed), "failed", time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for zero time, got %v", err)
	}
}

func TestVideoProcessingJobCancel(t *testing.T) {
	now := testJobTime()
	statuses := []VideoProcessingJobStatus{VideoJobQueued, VideoJobRetryWait, VideoJobRunning}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			startedAt := now
			job := &VideoProcessingJob{Status: status, AttemptCount: 1, MaxAttempts: 4, AvailableAt: now}
			if status == VideoJobQueued {
				job.AttemptCount = 0
			} else {
				job.StartedAt = &startedAt
			}
			if err := job.Cancel(now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if job.Status != VideoJobCancelled || job.FinishedAt == nil {
				t.Fatalf("unexpected cancelled job: %#v", job)
			}
		})
	}

	job := mustNewVideoJob(t, now)
	if err := job.Cancel(time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestVideoProcessingJobStatusAndFailureCodeValidation(t *testing.T) {
	if !VideoJobQueued.IsValid() || VideoProcessingJobStatus("invalid").IsValid() {
		t.Fatal("unexpected job status validation result")
	}
	if !VideoFailureCorrupt.IsValid() || VideoFailureCode("invalid").IsValid() {
		t.Fatal("unexpected failure code validation result")
	}
	if _, ok := normalizeVideoJobError(videoJobErrorWorkerTimeout, "worker timed out"); !ok {
		t.Fatal("worker_timeout must be accepted as an internal job error code")
	}
}

func testJobTime() time.Time {
	return time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC)
}

func mustNewVideoJob(t *testing.T, now time.Time) *VideoProcessingJob {
	t.Helper()
	job, err := NewVideoProcessingJob(1, now)
	if err != nil {
		t.Fatal(err)
	}
	return job
}
