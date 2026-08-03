package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

const (
	storageCleanupTemporaryCode    = "storage_unavailable"
	storageCleanupPermanentCode    = "delete_failed"
	storageCleanupTimeoutMessage   = "object storage deletion timed out"
	storageCleanupTemporaryMessage = "object storage is temporarily unavailable"
	storageCleanupPermanentMessage = "object storage deletion failed"
)

type StorageCleanupUsecaseConfig struct {
	RetryDelays  [3]time.Duration
	Timeout      time.Duration
	OrphanMinAge time.Duration
	ListLimit    int32
}

type IStorageCleanupUsecase interface {
	ProcessNext(ctx context.Context) (bool, error)
	RecoverTimedOutJobs(ctx context.Context, now time.Time, limit int) (int, error)
	DetectOrphanObjects(ctx context.Context, now time.Time) (int, error)
}

type storageCleanupUsecase struct {
	jobs         repository.IStorageCleanupJobRepository
	storage      repository.IObjectStorageRepository
	videos       repository.IVideoRepository
	retryDelays  [3]time.Duration
	timeout      time.Duration
	orphanMinAge time.Duration
	listLimit    int32
}

func NewStorageCleanupUsecase(
	jobs repository.IStorageCleanupJobRepository,
	storage repository.IObjectStorageRepository,
	videos repository.IVideoRepository,
	config StorageCleanupUsecaseConfig,
) (IStorageCleanupUsecase, error) {
	if jobs == nil || storage == nil || videos == nil || config.Timeout <= 0 || config.OrphanMinAge <= 0 || config.ListLimit < 1 || config.ListLimit > 1000 {
		return nil, fmt.Errorf("storage cleanup usecase configuration is invalid")
	}
	for _, delay := range config.RetryDelays {
		if delay <= 0 {
			return nil, fmt.Errorf("storage cleanup retry delay is invalid")
		}
	}

	return &storageCleanupUsecase{
		jobs:         jobs,
		storage:      storage,
		videos:       videos,
		retryDelays:  config.RetryDelays,
		timeout:      config.Timeout,
		orphanMinAge: config.OrphanMinAge,
		listLimit:    config.ListLimit,
	}, nil
}

func (u *storageCleanupUsecase) ProcessNext(ctx context.Context) (bool, error) {
	job, err := u.jobs.ClaimNext(ctx, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}
	if job.ID == 0 || strings.TrimSpace(job.ObjectKey) == "" {
		return true, entity.ErrCleanupJobConflict
	}

	deleteCtx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	if err := u.storage.Delete(deleteCtx, job.ObjectKey); err != nil && !errors.Is(err, entity.ErrObjectNotFound) {
		return true, u.recordDeleteFailure(ctx, job, err)
	}
	if err := u.jobs.MarkSucceeded(ctx, job.ID, time.Now().UTC()); err != nil {
		return true, err
	}

	return true, nil
}

func (u *storageCleanupUsecase) RecoverTimedOutJobs(ctx context.Context, now time.Time, limit int) (int, error) {
	if now.IsZero() || limit < 1 {
		return 0, entity.ErrInvalidInput
	}

	now = now.UTC()
	return u.jobs.RecoverTimedOut(ctx, repository.StorageCleanupRecoveryInput{
		TimeoutBefore: now.Add(-u.timeout),
		Now:           now,
		Limit:         limit,
		RetryDelays:   u.retryDelays,
	})
}

func (u *storageCleanupUsecase) DetectOrphanObjects(ctx context.Context, now time.Time) (int, error) {
	if now.IsZero() {
		return 0, entity.ErrInvalidInput
	}

	now = now.UTC()
	cutoff := now.Add(-u.orphanMinAge)
	registered := 0
	var cursor *string

	for {
		page, err := u.storage.ListManagedObjects(ctx, cursor, u.listLimit)
		if err != nil {
			return registered, err
		}

		for _, object := range page.Items {
			if strings.TrimSpace(object.ObjectKey) == "" || object.LastModifiedAt.IsZero() || object.LastModifiedAt.UTC().After(cutoff) {
				continue
			}

			referenced, err := u.videos.IsObjectReferenced(ctx, object.ObjectKey)
			if err != nil {
				return registered, err
			}
			if referenced {
				continue
			}

			job, err := entity.NewStorageCleanupJob(nil, object.ObjectKey, entity.StorageAssetUnknown, entity.StorageCleanupOrphanDetected, now)
			if err != nil {
				return registered, err
			}
			if err := u.jobs.Create(ctx, job); err != nil {
				return registered, err
			}
			registered++
		}

		if !page.HasMore {
			return registered, nil
		}
		if page.NextCursor == nil || strings.TrimSpace(*page.NextCursor) == "" {
			return registered, fmt.Errorf("managed object page has no next cursor")
		}
		if cursor != nil && *cursor == *page.NextCursor {
			return registered, fmt.Errorf("managed object cursor did not advance")
		}

		next := *page.NextCursor
		cursor = &next
	}
}

func (u *storageCleanupUsecase) recordDeleteFailure(ctx context.Context, job *entity.StorageCleanupJob, err error) error {
	if job == nil || job.ID == 0 || job.AttemptCount < 1 || job.MaxAttempts < 1 || job.AttemptCount > job.MaxAttempts {
		return entity.ErrCleanupJobConflict
	}

	now := time.Now().UTC()
	code, message, retryable := classifyCleanupError(err)

	if retryable && job.AttemptCount < job.MaxAttempts {
		retryIndex := job.AttemptCount - 1
		if retryIndex < 0 || retryIndex >= len(u.retryDelays) {
			return entity.ErrCleanupJobConflict
		}
		return u.jobs.ScheduleRetry(ctx, job.ID, now.Add(u.retryDelays[retryIndex]), code, message, now)
	}

	return u.jobs.MarkFailed(ctx, job.ID, code, message, now)
}

func classifyCleanupError(err error) (string, string, bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return storageCleanupTemporaryCode, storageCleanupTimeoutMessage, true
	case errors.Is(err, entity.ErrStorageUnavailable):
		return storageCleanupTemporaryCode, storageCleanupTemporaryMessage, true
	default:
		return storageCleanupPermanentCode, storageCleanupPermanentMessage, false
	}
}
