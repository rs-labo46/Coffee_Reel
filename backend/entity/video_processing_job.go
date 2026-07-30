package entity

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	videoProcessingMaxAttempts = 4
	jobErrorMessageMaxRunes    = 500
	videoJobErrorWorkerTimeout = "worker_timeout"
)

type VideoProcessingJobStatus string

const (
	VideoJobQueued    VideoProcessingJobStatus = "queued"
	VideoJobRunning   VideoProcessingJobStatus = "running"
	VideoJobRetryWait VideoProcessingJobStatus = "retry_wait"
	VideoJobSucceeded VideoProcessingJobStatus = "succeeded"
	VideoJobFailed    VideoProcessingJobStatus = "failed"
	VideoJobCancelled VideoProcessingJobStatus = "cancelled"
)

type VideoFailureCode string

const (
	VideoFailureInvalidFormat      VideoFailureCode = "invalid_format"
	VideoFailureCorrupt            VideoFailureCode = "video_corrupt"
	VideoFailureDurationExceeded   VideoFailureCode = "duration_exceeded"
	VideoFailureSizeExceeded       VideoFailureCode = "size_exceeded"
	VideoFailureResolutionExceeded VideoFailureCode = "resolution_exceeded"
	VideoFailureInvalidAspectRatio VideoFailureCode = "invalid_aspect_ratio"
	VideoFailureFrameRateExceeded  VideoFailureCode = "frame_rate_exceeded"
	VideoFailureVideoTrackMissing  VideoFailureCode = "video_track_missing"
	VideoFailureProcessingFailed   VideoFailureCode = "processing_failed"
	VideoFailureStorageUnavailable VideoFailureCode = "storage_unavailable"
)

type VideoProcessingJob struct {
	ID               uint64                   `json:"id" gorm:"primaryKey"`
	VideoID          uint64                   `json:"video_id" gorm:"not null;uniqueIndex:uq_video_processing_jobs_video_id"`
	Status           VideoProcessingJobStatus `json:"status" gorm:"type:varchar(32);not null"`
	AttemptCount     int                      `json:"attempt_count" gorm:"not null;default:0"`
	MaxAttempts      int                      `json:"max_attempts" gorm:"not null;default:4"`
	AvailableAt      time.Time                `json:"available_at" gorm:"not null"`
	StartedAt        *time.Time               `json:"started_at"`
	FinishedAt       *time.Time               `json:"finished_at"`
	LastErrorCode    string                   `json:"-" gorm:"type:varchar(64);not null"`
	LastErrorMessage string                   `json:"-" gorm:"type:varchar(500);not null"`
	CreatedAt        time.Time                `json:"created_at" gorm:"not null"`
	UpdatedAt        time.Time                `json:"updated_at" gorm:"not null"`
}

func (s VideoProcessingJobStatus) IsValid() bool {
	switch s {
	case VideoJobQueued, VideoJobRunning, VideoJobRetryWait, VideoJobSucceeded, VideoJobFailed, VideoJobCancelled:
		return true
	default:
		return false
	}
}

func (c VideoFailureCode) IsValid() bool {
	switch c {
	case VideoFailureInvalidFormat,
		VideoFailureCorrupt,
		VideoFailureDurationExceeded,
		VideoFailureSizeExceeded,
		VideoFailureResolutionExceeded,
		VideoFailureInvalidAspectRatio,
		VideoFailureFrameRateExceeded,
		VideoFailureVideoTrackMissing,
		VideoFailureProcessingFailed,
		VideoFailureStorageUnavailable:
		return true
	default:
		return false
	}
}

func NewVideoProcessingJob(videoID uint64, now time.Time) (*VideoProcessingJob, error) {
	if videoID == 0 || now.IsZero() {
		return nil, ErrInvalidInput
	}
	now = now.UTC()
	return &VideoProcessingJob{
		VideoID:          videoID,
		Status:           VideoJobQueued,
		AttemptCount:     0,
		MaxAttempts:      videoProcessingMaxAttempts,
		AvailableAt:      now,
		LastErrorCode:    "",
		LastErrorMessage: "",
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (j *VideoProcessingJob) Claim(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	now = now.UTC()
	if j == nil || (j.Status != VideoJobQueued && j.Status != VideoJobRetryWait) || j.AttemptCount < 0 || j.MaxAttempts != videoProcessingMaxAttempts || j.AttemptCount >= j.MaxAttempts || j.AvailableAt.IsZero() || now.Before(j.AvailableAt.UTC()) {
		return ErrProcessingJobConflict
	}
	j.Status = VideoJobRunning
	j.AttemptCount++
	j.StartedAt = &now
	j.FinishedAt = nil
	j.UpdatedAt = now
	return nil
}

func (j *VideoProcessingJob) ScheduleRetry(availableAt time.Time, code, message string, now time.Time) error {
	if now.IsZero() || availableAt.IsZero() {
		return ErrInvalidInput
	}
	now = now.UTC()
	availableAt = availableAt.UTC()
	message, ok := normalizeVideoJobError(code, message)
	if !ok {
		return ErrInvalidInput
	}
	if j == nil || j.Status != VideoJobRunning || j.AttemptCount < 1 || j.AttemptCount >= j.MaxAttempts || j.MaxAttempts != videoProcessingMaxAttempts || j.StartedAt == nil || j.FinishedAt != nil || !availableAt.After(now) {
		return ErrProcessingJobConflict
	}
	j.Status = VideoJobRetryWait
	j.AvailableAt = availableAt
	j.FinishedAt = nil
	j.LastErrorCode = code
	j.LastErrorMessage = message
	j.UpdatedAt = now
	return nil
}

func (j *VideoProcessingJob) MarkSucceeded(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if j == nil || j.Status != VideoJobRunning || j.AttemptCount < 1 || j.StartedAt == nil || j.FinishedAt != nil {
		return ErrProcessingJobConflict
	}
	now = now.UTC()
	j.Status = VideoJobSucceeded
	j.FinishedAt = &now
	j.LastErrorCode = ""
	j.LastErrorMessage = ""
	j.UpdatedAt = now
	return nil
}

func (j *VideoProcessingJob) MarkFailed(code, message string, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	message, ok := normalizeVideoJobError(code, message)
	if !ok {
		return ErrInvalidInput
	}
	if j == nil || j.Status != VideoJobRunning || j.AttemptCount < 1 || j.StartedAt == nil || j.FinishedAt != nil {
		return ErrProcessingJobConflict
	}
	now = now.UTC()
	j.Status = VideoJobFailed
	j.FinishedAt = &now
	j.LastErrorCode = code
	j.LastErrorMessage = message
	j.UpdatedAt = now
	return nil
}

func (j *VideoProcessingJob) Cancel(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if j == nil || (j.Status != VideoJobQueued && j.Status != VideoJobRetryWait && j.Status != VideoJobRunning) || j.FinishedAt != nil {
		return ErrProcessingJobConflict
	}
	now = now.UTC()
	j.Status = VideoJobCancelled
	j.FinishedAt = &now
	j.UpdatedAt = now
	return nil
}

func normalizeVideoJobError(code, message string) (string, bool) {
	if (!VideoFailureCode(code).IsValid() && code != videoJobErrorWorkerTimeout) || !utf8.ValidString(message) {
		return "", false
	}
	message = strings.TrimSpace(message)
	count := utf8.RuneCountInString(message)
	if count < 1 || count > jobErrorMessageMaxRunes {
		return "", false
	}
	return message, true
}
