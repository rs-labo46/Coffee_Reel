package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"coffee-reel/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	processingRetryDelayCount = 3
	processingTimeoutCode     = "worker_timeout"
	processingTimeoutMessage  = "worker execution timed out"
)

type IVideoProcessingJobRepository interface {
	ClaimNext(ctx context.Context, now time.Time) (*ProcessingClaim, error)
	ScheduleRetry(ctx context.Context, jobID uint64, availableAt time.Time, code, message string, now time.Time) error
	Cancel(ctx context.Context, jobID uint64, now time.Time) error
	RecoverTimedOut(ctx context.Context, input ProcessingRecoveryInput) (int, error)
}

type ProcessingClaim struct {
	Job        *entity.VideoProcessingJob
	Video      *entity.Video
	SourceMeta *entity.SourceVideoMeta
}

type ProcessingRecoveryInput struct {
	TimeoutBefore time.Time
	Now           time.Time
	Limit         int
	RetryDelays   [processingRetryDelayCount]time.Duration
}

type videoProcessingJobRepository struct {
	db *gorm.DB
}

func NewVideoProcessingJobRepository(db *gorm.DB) IVideoProcessingJobRepository {
	return &videoProcessingJobRepository{db: db}
}

func (r *videoProcessingJobRepository) ClaimNext(ctx context.Context, now time.Time) (*ProcessingClaim, error) {
	if now.IsZero() {
		return nil, entity.ErrInvalidInput
	}
	now = now

	var claim *ProcessingClaim
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.VideoProcessingJob

		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ? AND available_at <= ? AND attempt_count < max_attempts", []entity.VideoProcessingJobStatus{entity.VideoJobQueued, entity.VideoJobRetryWait}, now).Order("available_at ASC").Order("id ASC").Take(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim next processing job: %w", err)
		}

		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", job.VideoID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock processing video: %w", err)
		}

		if video.DeletedAt != nil {
			if err := job.Cancel(now); err != nil {
				return err
			}
			if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).Select("status", "finished_at", "updated_at").Updates(&job).Error; err != nil {
				return fmt.Errorf("cancel deleted processing job: %w", err)
			}
			return nil
		}

		if job.Status == entity.VideoJobQueued {
			if err := video.StartProcessing(now); err != nil {
				return err
			}
			if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select("processing_status", "updated_at").Updates(&video).Error; err != nil {
				return fmt.Errorf("save processing video start: %w", err)
			}
		} else if video.ProcessingStatus != entity.VideoProcessingProcessing || video.PublishStatus != entity.VideoPublishPrivate {
			return entity.ErrVideoStateConflict
		}

		if err := job.Claim(now); err != nil {
			return err
		}
		if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).Select("status", "attempt_count", "started_at", "finished_at", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save claimed processing job: %w", err)
		}

		var source entity.SourceVideoMeta
		var sourceMeta *entity.SourceVideoMeta

		if err := tx.Where("video_id = ?", video.ID).Take(&source).Error; err == nil {
			sourceMeta = &source
			video.SourceMeta = &source
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find processing source meta: %w", err)
		}

		claim = &ProcessingClaim{
			Job:        &job,
			Video:      &video,
			SourceMeta: sourceMeta,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return claim, nil
}

func (r *videoProcessingJobRepository) ScheduleRetry(ctx context.Context, jobID uint64, availableAt time.Time, code, message string, now time.Time) error {
	if jobID == 0 || availableAt.IsZero() || now.IsZero() {
		return entity.ErrInvalidInput
	}

	safeMessage, err := normalizeProcessingJobMessage(message)
	if err != nil {
		return err
	}

	availableAt = availableAt
	now = now

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.VideoProcessingJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrProcessingJobNotFound
			}
			return fmt.Errorf("lock job for retry: %w", err)
		}

		if err := job.ScheduleRetry(availableAt, code, safeMessage, now); err != nil {
			return err
		}

		if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).Select("status", "available_at", "last_error_code", "last_error_message", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save processing retry: %w", err)
		}

		return nil
	})
}

func (r *videoProcessingJobRepository) Cancel(ctx context.Context, jobID uint64, now time.Time) error {
	if jobID == 0 || now.IsZero() {
		return entity.ErrInvalidInput
	}
	now = now

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.VideoProcessingJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrProcessingJobNotFound
			}
			return fmt.Errorf("lock job for cancellation: %w", err)
		}

		switch job.Status {
		case entity.VideoJobCancelled,
			entity.VideoJobSucceeded,
			entity.VideoJobFailed:
			return nil
		}

		if err := job.Cancel(now); err != nil {
			return err
		}

		if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).Select("status", "finished_at", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save cancelled processing job: %w", err)
		}

		return nil
	})
}

func (r *videoProcessingJobRepository) RecoverTimedOut(ctx context.Context, input ProcessingRecoveryInput) (int, error) {
	if input.TimeoutBefore.IsZero() || input.Now.IsZero() || !input.TimeoutBefore.Before(input.Now) || input.Limit < 1 {
		return 0, entity.ErrInvalidInput
	}

	for _, delay := range input.RetryDelays {
		if delay <= 0 {
			return 0, entity.ErrInvalidInput
		}
	}

	timeoutBefore := input.TimeoutBefore
	now := input.Now
	count := 0

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []entity.VideoProcessingJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND started_at IS NOT NULL AND started_at <= ?", entity.VideoJobRunning, timeoutBefore).Order("started_at ASC").Order("id ASC").Limit(input.Limit).Find(&jobs).Error; err != nil {
			return fmt.Errorf("claim timed out processing jobs: %w", err)
		}

		for i := range jobs {
			job := &jobs[i]

			if job.Status != entity.VideoJobRunning || job.AttemptCount < 1 || job.AttemptCount > job.MaxAttempts {
				return entity.ErrProcessingJobConflict
			}

			var video entity.Video
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", job.VideoID).Take(&video).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return entity.ErrVideoNotFound
				}
				return fmt.Errorf("lock timed out processing video: %w", err)
			}

			switch {
			case video.DeletedAt != nil:
				if err := job.Cancel(now); err != nil {
					return err
				}

			case job.AttemptCount < job.MaxAttempts:
				if video.ProcessingStatus != entity.VideoProcessingProcessing || video.PublishStatus != entity.VideoPublishPrivate {
					return entity.ErrVideoStateConflict
				}

				retryIndex := job.AttemptCount - 1
				if retryIndex < 0 || retryIndex >= len(input.RetryDelays) {
					return entity.ErrProcessingJobConflict
				}

				availableAt := now.Add(input.RetryDelays[retryIndex])
				if err := job.ScheduleRetry(availableAt, processingTimeoutCode, processingTimeoutMessage, now); err != nil {
					return err
				}

			default:
				if err := video.FailProcessing(now); err != nil {
					return err
				}

				if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select("processing_status", "publish_status", "updated_at").Updates(&video).Error; err != nil {
					return fmt.Errorf("save recovered failed video: %w", err)
				}

				if err := job.MarkFailed(processingTimeoutCode, processingTimeoutMessage, now); err != nil {
					return err
				}
			}

			if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).Select("status", "available_at", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(job).Error; err != nil {
				return fmt.Errorf("save recovered processing job: %w", err)
			}

			count++
		}

		return nil
	})

	return count, err
}

func normalizeProcessingJobMessage(message string) (string, error) {
	if !utf8.ValidString(message) {
		return "", entity.ErrInvalidInput
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return "", entity.ErrInvalidInput
	}

	runes := []rune(message)
	if len(runes) > 500 {
		message = string(runes[:500])
	}

	return message, nil
}
