package entity

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	storageCleanupMaxAttempts   = 4
	cleanupErrorCodeMaxRunes    = 64
	cleanupErrorMessageMaxRunes = 500
)

type StorageAssetType string

type StorageCleanupCause string

type StorageCleanupStatus string

const (
	StorageAssetOriginal  StorageAssetType = "original"
	StorageAssetProcessed StorageAssetType = "processed"
	StorageAssetThumbnail StorageAssetType = "thumbnail"
	StorageAssetUnknown   StorageAssetType = "unknown"
)

const (
	StorageCleanupVideoDelete    StorageCleanupCause = "video_delete"
	StorageCleanupUploadExpired  StorageCleanupCause = "upload_expired"
	StorageCleanupProcessFailed  StorageCleanupCause = "process_failed"
	StorageCleanupRollback       StorageCleanupCause = "rollback_cleanup"
	StorageCleanupOrphanDetected StorageCleanupCause = "orphan_detected"
)

const (
	StorageCleanupQueued    StorageCleanupStatus = "queued"
	StorageCleanupRunning   StorageCleanupStatus = "running"
	StorageCleanupRetryWait StorageCleanupStatus = "retry_wait"
	StorageCleanupSucceeded StorageCleanupStatus = "succeeded"
	StorageCleanupFailed    StorageCleanupStatus = "failed"
)

type StorageCleanupJob struct {
	ID               uint64               `json:"id" gorm:"primaryKey"`
	VideoID          *uint64              `json:"video_id"`
	ObjectKey        string               `json:"-" gorm:"type:varchar(1024);not null"`
	AssetType        StorageAssetType     `json:"asset_type" gorm:"type:varchar(32);not null"`
	Cause            StorageCleanupCause  `json:"cause" gorm:"type:varchar(32);not null"`
	Status           StorageCleanupStatus `json:"status" gorm:"type:varchar(32);not null"`
	AttemptCount     int                  `json:"attempt_count" gorm:"not null;default:0"`
	MaxAttempts      int                  `json:"max_attempts" gorm:"not null;default:4"`
	AvailableAt      time.Time            `json:"available_at" gorm:"not null"`
	StartedAt        *time.Time           `json:"started_at"`
	FinishedAt       *time.Time           `json:"finished_at"`
	LastErrorCode    string               `json:"-" gorm:"type:varchar(64);not null"`
	LastErrorMessage string               `json:"-" gorm:"type:varchar(500);not null"`
	CreatedAt        time.Time            `json:"created_at" gorm:"not null"`
	UpdatedAt        time.Time            `json:"updated_at" gorm:"not null"`
}

func NewStorageCleanupJob(videoID *uint64, objectKey string, assetType StorageAssetType, cause StorageCleanupCause, now time.Time) (*StorageCleanupJob, error) {
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" || now.IsZero() || !assetType.IsValid() || !cause.IsValid() {
		return nil, ErrInvalidInput
	}
	if videoID != nil && *videoID == 0 {
		return nil, ErrInvalidInput
	}
	now = now.UTC()
	return &StorageCleanupJob{
		VideoID:          videoID,
		ObjectKey:        objectKey,
		AssetType:        assetType,
		Cause:            cause,
		Status:           StorageCleanupQueued,
		AttemptCount:     0,
		MaxAttempts:      storageCleanupMaxAttempts,
		AvailableAt:      now,
		LastErrorCode:    "",
		LastErrorMessage: "",
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (v StorageAssetType) IsValid() bool {
	return v == StorageAssetOriginal || v == StorageAssetProcessed || v == StorageAssetThumbnail || v == StorageAssetUnknown
}

func (v StorageCleanupCause) IsValid() bool {
	return v == StorageCleanupVideoDelete || v == StorageCleanupUploadExpired || v == StorageCleanupProcessFailed || v == StorageCleanupRollback || v == StorageCleanupOrphanDetected
}

func (v StorageCleanupStatus) IsValid() bool {
	return v == StorageCleanupQueued || v == StorageCleanupRunning || v == StorageCleanupRetryWait || v == StorageCleanupSucceeded || v == StorageCleanupFailed
}

func (j *StorageCleanupJob) Claim(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	now = now.UTC()
	if j == nil || (j.Status != StorageCleanupQueued && j.Status != StorageCleanupRetryWait) || j.AttemptCount < 0 || j.MaxAttempts != storageCleanupMaxAttempts || j.AttemptCount >= j.MaxAttempts || j.AvailableAt.IsZero() || now.Before(j.AvailableAt.UTC()) {
		return ErrCleanupJobConflict
	}
	j.Status = StorageCleanupRunning
	j.AttemptCount++
	j.StartedAt = &now
	j.FinishedAt = nil
	j.UpdatedAt = now
	return nil
}

func (j *StorageCleanupJob) ScheduleRetry(availableAt time.Time, code, message string, now time.Time) error {
	if now.IsZero() || availableAt.IsZero() {
		return ErrInvalidInput
	}
	now = now.UTC()
	availableAt = availableAt.UTC()
	code, message, ok := normalizeCleanupError(code, message)
	if !ok {
		return ErrInvalidInput
	}
	if j == nil || j.Status != StorageCleanupRunning || j.AttemptCount < 1 || j.AttemptCount >= j.MaxAttempts || j.MaxAttempts != storageCleanupMaxAttempts || j.StartedAt == nil || j.FinishedAt != nil || !availableAt.After(now) {
		return ErrCleanupJobConflict
	}
	j.Status = StorageCleanupRetryWait
	j.AvailableAt = availableAt
	j.FinishedAt = nil
	j.LastErrorCode = code
	j.LastErrorMessage = message
	j.UpdatedAt = now
	return nil
}

func (j *StorageCleanupJob) MarkSucceeded(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if j == nil || j.Status != StorageCleanupRunning || j.AttemptCount < 1 || j.StartedAt == nil || j.FinishedAt != nil {
		return ErrCleanupJobConflict
	}
	now = now.UTC()
	j.Status = StorageCleanupSucceeded
	j.FinishedAt = &now
	j.LastErrorCode = ""
	j.LastErrorMessage = ""
	j.UpdatedAt = now
	return nil
}

func (j *StorageCleanupJob) MarkFailed(code, message string, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	code, message, ok := normalizeCleanupError(code, message)
	if !ok {
		return ErrInvalidInput
	}
	if j == nil || j.Status != StorageCleanupRunning || j.AttemptCount < 1 || j.StartedAt == nil || j.FinishedAt != nil {
		return ErrCleanupJobConflict
	}
	now = now.UTC()
	j.Status = StorageCleanupFailed
	j.FinishedAt = &now
	j.LastErrorCode = code
	j.LastErrorMessage = message
	j.UpdatedAt = now
	return nil
}

func normalizeCleanupError(code, message string) (string, string, bool) {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if !isSafeCleanupErrorCode(code) || !utf8.ValidString(message) {
		return "", "", false
	}
	count := utf8.RuneCountInString(message)
	if count < 1 || count > cleanupErrorMessageMaxRunes {
		return "", "", false
	}
	return code, message, true
}

func isSafeCleanupErrorCode(code string) bool {
	if !utf8.ValidString(code) {
		return false
	}
	count := utf8.RuneCountInString(code)
	if count < 1 || count > cleanupErrorCodeMaxRunes {
		return false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
