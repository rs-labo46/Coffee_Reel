package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"coffee-reel/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	storageCleanupRetryDelayCount = 3
	storageCleanupTimeoutCode     = "worker_timeout"
	storageCleanupTimeoutMessage  = "cleanup worker timed out"
)

type IStorageCleanupJobRepository interface {
	Create(ctx context.Context, job *entity.StorageCleanupJob) error
	ClaimNext(ctx context.Context, now time.Time) (*entity.StorageCleanupJob, error)
	ScheduleRetry(ctx context.Context, jobID uint64, availableAt time.Time, code, message string, now time.Time) error
	MarkSucceeded(ctx context.Context, jobID uint64, now time.Time) error
	MarkFailed(ctx context.Context, jobID uint64, code, message string, now time.Time) error
	RecoverTimedOut(ctx context.Context, input StorageCleanupRecoveryInput) (int, error)
}

type StorageCleanupRecoveryInput struct {
	TimeoutBefore time.Time
	Now           time.Time
	Limit         int
	RetryDelays   [storageCleanupRetryDelayCount]time.Duration
}

type storageCleanupJobRepository struct {
	db *gorm.DB
}

func NewStorageCleanupJobRepository(db *gorm.DB) IStorageCleanupJobRepository {
	return &storageCleanupJobRepository{db: db}
}

func (r *storageCleanupJobRepository) Create(ctx context.Context, job *entity.StorageCleanupJob) error {
	if job == nil {
		return entity.ErrInvalidInput
	}

	if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
		if isConstraintViolation(err, "uq_cleanup_unfinished_object") {
			return nil
		}
		return fmt.Errorf("create cleanup job: %w", err)
	}

	return nil
}

func (r *storageCleanupJobRepository) ClaimNext(ctx context.Context, now time.Time) (*entity.StorageCleanupJob, error) {
	if now.IsZero() {
		return nil, entity.ErrInvalidInput
	}
	now = now.UTC()

	var output *entity.StorageCleanupJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.StorageCleanupJob

		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ? AND available_at <= ? AND attempt_count < max_attempts", []entity.StorageCleanupStatus{entity.StorageCleanupQueued, entity.StorageCleanupRetryWait}, now).Order("available_at ASC").Order("id ASC").Take(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim cleanup job: %w", err)
		}

		if err := job.Claim(now); err != nil {
			return err
		}
		if err := tx.Model(&entity.StorageCleanupJob{}).Where("id = ?", job.ID).Select("status", "attempt_count", "started_at", "finished_at", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save claimed cleanup job: %w", err)
		}

		jobCopy := job
		output = &jobCopy
		return nil
	})
	if err != nil {
		return nil, err
	}

	return output, nil
}

func (r *storageCleanupJobRepository) ScheduleRetry(ctx context.Context, jobID uint64, availableAt time.Time, code, message string, now time.Time) error {
	if jobID == 0 || availableAt.IsZero() || now.IsZero() {
		return entity.ErrInvalidInput
	}
	availableAt = availableAt.UTC()
	now = now.UTC()

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.StorageCleanupJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrCleanupJobNotFound
			}
			return fmt.Errorf("lock cleanup job for retry: %w", err)
		}

		if err := job.ScheduleRetry(availableAt, code, message, now); err != nil {
			return err
		}
		if err := tx.Model(&entity.StorageCleanupJob{}).Where("id = ?", job.ID).Select("status", "available_at", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save cleanup job retry: %w", err)
		}

		return nil
	})
}

func (r *storageCleanupJobRepository) MarkSucceeded(ctx context.Context, jobID uint64, now time.Time) error {
	if jobID == 0 || now.IsZero() {
		return entity.ErrInvalidInput
	}
	now = now.UTC()

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.StorageCleanupJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrCleanupJobNotFound
			}
			return fmt.Errorf("lock cleanup job for success: %w", err)
		}

		if err := job.MarkSucceeded(now); err != nil {
			return err
		}
		if err := tx.Model(&entity.StorageCleanupJob{}).Where("id = ?", job.ID).Select("status", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save succeeded cleanup job: %w", err)
		}

		return nil
	})
}

func (r *storageCleanupJobRepository) MarkFailed(ctx context.Context, jobID uint64, code, message string, now time.Time) error {
	if jobID == 0 || now.IsZero() {
		return entity.ErrInvalidInput
	}
	now = now.UTC()

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.StorageCleanupJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrCleanupJobNotFound
			}
			return fmt.Errorf("lock cleanup job for failure: %w", err)
		}

		if err := job.MarkFailed(code, message, now); err != nil {
			return err
		}
		if err := tx.Model(&entity.StorageCleanupJob{}).Where("id = ?", job.ID).Select("status", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save failed cleanup job: %w", err)
		}

		return nil
	})
}

func (r *storageCleanupJobRepository) RecoverTimedOut(ctx context.Context, input StorageCleanupRecoveryInput) (int, error) {
	if input.TimeoutBefore.IsZero() || input.Now.IsZero() || !input.TimeoutBefore.Before(input.Now) || input.Limit < 1 {
		return 0, entity.ErrInvalidInput
	}
	for _, delay := range input.RetryDelays {
		if delay <= 0 {
			return 0, entity.ErrInvalidInput
		}
	}

	timeoutBefore := input.TimeoutBefore.UTC()
	now := input.Now.UTC()
	count := 0

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []entity.StorageCleanupJob

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND started_at IS NOT NULL AND started_at <= ?", entity.StorageCleanupRunning, timeoutBefore).Order("started_at ASC").Order("id ASC").Limit(input.Limit).Find(&jobs).Error; err != nil {
			return fmt.Errorf("claim timed out cleanup jobs: %w", err)
		}

		for i := range jobs {
			job := &jobs[i]

			if job.Status != entity.StorageCleanupRunning || job.AttemptCount < 1 || job.MaxAttempts != storageCleanupRetryDelayCount+1 || job.AttemptCount > job.MaxAttempts {
				return entity.ErrCleanupJobConflict
			}

			if job.AttemptCount < job.MaxAttempts {
				retryIndex := job.AttemptCount - 1
				if retryIndex < 0 || retryIndex >= len(input.RetryDelays) {
					return entity.ErrCleanupJobConflict
				}

				availableAt := now.Add(input.RetryDelays[retryIndex])
				if err := job.ScheduleRetry(availableAt, storageCleanupTimeoutCode, storageCleanupTimeoutMessage, now); err != nil {
					return err
				}
			} else {
				if err := job.MarkFailed(storageCleanupTimeoutCode, storageCleanupTimeoutMessage, now); err != nil {
					return err
				}
			}

			if err := tx.Model(&entity.StorageCleanupJob{}).Where("id = ?", job.ID).Select("status", "available_at", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(job).Error; err != nil {
				return fmt.Errorf("save recovered cleanup job: %w", err)
			}

			count++
		}

		return nil
	})

	return count, err
}
