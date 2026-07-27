package entity

import "time"

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
	Status           VideoProcessingJobStatus `json:"status" gorm:"type:varchar(32);not null;index:idx_video_processing_jobs_claim,priority:1"`
	AttemptCount     int                      `json:"attempt_count" gorm:"not null;default:0"`
	MaxAttempts      int                      `json:"max_attempts" gorm:"not null;default:4"`
	AvailableAt      time.Time                `json:"available_at" gorm:"not null;index:idx_video_processing_jobs_claim,priority:2"`
	StartedAt        *time.Time               `json:"started_at" gorm:"index:idx_video_processing_jobs_timeout"`
	FinishedAt       *time.Time               `json:"finished_at"`
	LastErrorCode    string                   `json:"-" gorm:"type:varchar(64);not null"`
	LastErrorMessage string                   `json:"-" gorm:"type:varchar(500);not null"`
	CreatedAt        time.Time                `json:"created_at" gorm:"not null"`
	UpdatedAt        time.Time                `json:"updated_at" gorm:"not null"`
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
		MaxAttempts:      4,
		AvailableAt:      now,
		LastErrorCode:    "",
		LastErrorMessage: "",
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (j *VideoProcessingJob) Claim(now time.Time) error {
	if j == nil || (j.Status != VideoJobQueued && j.Status != VideoJobRetryWait) || j.AttemptCount >= j.MaxAttempts || now.UTC().Before(j.AvailableAt.UTC()) {
		return ErrProcessingJobConflict
	}
	now = now.UTC()
	j.Status = VideoJobRunning
	j.AttemptCount++
	j.StartedAt = &now
	j.FinishedAt = nil
	j.UpdatedAt = now
	return nil
}

func (j *VideoProcessingJob) ScheduleRetry(availableAt time.Time, code, message string, now time.Time) error {
	if j == nil || j.Status != VideoJobRunning || j.AttemptCount >= j.MaxAttempts || !availableAt.After(now) {
		return ErrProcessingJobConflict
	}
	j.Status = VideoJobRetryWait
	j.AvailableAt = availableAt.UTC()
	j.LastErrorCode = code
	j.LastErrorMessage = message
	j.UpdatedAt = now.UTC()
	return nil
}

func (j *VideoProcessingJob) MarkSucceeded(now time.Time) error {
	if j == nil || j.Status != VideoJobRunning {
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
	if j == nil || j.Status != VideoJobRunning {
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
	if j == nil || (j.Status != VideoJobQueued && j.Status != VideoJobRetryWait && j.Status != VideoJobRunning) {
		return ErrProcessingJobConflict
	}
	now = now.UTC()
	j.Status = VideoJobCancelled
	j.FinishedAt = &now
	j.UpdatedAt = now
	return nil
}
