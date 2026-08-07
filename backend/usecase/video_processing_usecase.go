package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

const (
	videoProcessingTimeout     = 5 * time.Minute
	processingObjectRandomSize = 16
)

type VideoProcessingUsecaseConfig struct {
	RetryDelays   [3]time.Duration
	ManagedPrefix string
}

type IVideoProcessingUsecase interface {
	ProcessNext(ctx context.Context) (bool, error)
	ExpireUploads(ctx context.Context, now time.Time, limit int) (int, error)
	RecoverTimedOutJobs(ctx context.Context, now time.Time, limit int) (int, error)
	DeleteExpiredIdempotencyRecords(ctx context.Context, now time.Time, limit int) (int64, error)
}

type videoProcessingUsecase struct {
	videos        repository.IVideoRepository
	jobs          repository.IVideoProcessingJobRepository
	storage       repository.IObjectStorageRepository
	media         repository.IMediaRepository
	cleanupJobs   repository.IStorageCleanupJobRepository
	retryDelays   [3]time.Duration
	managedPrefix string
}

type processingFailure struct {
	Code      entity.VideoFailureCode
	Retryable bool
	Message   string
}

func NewVideoProcessingUsecase(
	videos repository.IVideoRepository,
	jobs repository.IVideoProcessingJobRepository,
	storage repository.IObjectStorageRepository,
	media repository.IMediaRepository,
	cleanupJobs repository.IStorageCleanupJobRepository,
	config VideoProcessingUsecaseConfig,
) (IVideoProcessingUsecase, error) {
	managedPrefix := normalizeUsecaseManagedPrefix(config.ManagedPrefix)
	if videos == nil || jobs == nil || storage == nil || media == nil || cleanupJobs == nil || managedPrefix != "videos/" {
		return nil, fmt.Errorf("video processing usecase configuration is invalid")
	}
	for _, delay := range config.RetryDelays {
		if delay <= 0 {
			return nil, fmt.Errorf("video processing retry delay is invalid")
		}
	}

	return &videoProcessingUsecase{
		videos:        videos,
		jobs:          jobs,
		storage:       storage,
		media:         media,
		cleanupJobs:   cleanupJobs,
		retryDelays:   config.RetryDelays,
		managedPrefix: managedPrefix,
	}, nil
}

func (u *videoProcessingUsecase) ProcessNext(ctx context.Context) (bool, error) {
	claim, err := u.jobs.ClaimNext(ctx, time.Now())
	if err != nil {
		return false, err
	}
	if claim == nil {
		return false, nil
	}
	if claim.Job == nil || claim.Video == nil || claim.Job.ID == 0 || claim.Video.ID == 0 || claim.Job.VideoID != claim.Video.ID {
		return true, entity.ErrProcessingJobConflict
	}

	workDir, err := os.MkdirTemp("", fmt.Sprintf("coffee-reel-video-%d-", claim.Job.ID))
	if err != nil {
		return true, u.recordFailure(ctx, claim, processingFailure{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: true,
			Message:   "temporary work directory could not be created",
		}, "", "")
	}
	defer os.RemoveAll(workDir)

	processCtx, cancel := context.WithTimeout(ctx, videoProcessingTimeout)
	defer cancel()

	sourcePath := filepath.Join(workDir, "source.bin")
	outputPath := filepath.Join(workDir, "output.mp4")
	thumbnailPath := filepath.Join(workDir, "thumbnail.jpg")

	if err := u.storage.Download(processCtx, claim.Video.OriginalObjectKey, sourcePath); err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}

	sourceMeta := claim.SourceMeta
	if sourceMeta == nil {
		probed, err := u.media.Probe(processCtx, sourcePath)
		if err != nil {
			return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
		}
		if err := u.videos.RecordSourceValidation(ctx, claim.Video.ID, probed, time.Now()); err != nil {
			if errors.Is(err, entity.ErrVideoNotFound) {
				return true, u.jobs.Cancel(ctx, claim.Job.ID, time.Now())
			}
			return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
		}
		sourceMeta = &probed
	}

	if err := u.media.Transcode(processCtx, sourcePath, outputPath, sourceMeta.HasAudio); err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}
	if err := u.media.GenerateThumbnail(processCtx, sourcePath, thumbnailPath); err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}

	outputMeta, err := u.media.ProbeOutput(processCtx, outputPath)
	if err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}

	videoObjectKey, err := buildProcessingObjectKey(u.managedPrefix, claim.Video.ID, "output", "mp4")
	if err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}
	thumbnailObjectKey, err := buildProcessingObjectKey(u.managedPrefix, claim.Video.ID, "thumbnail", "jpg")
	if err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}

	if err := u.storage.UploadProcessed(processCtx, videoObjectKey, outputPath); err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), "", "")
	}
	if err := u.storage.UploadThumbnail(processCtx, thumbnailObjectKey, thumbnailPath); err != nil {
		return true, u.recordFailure(ctx, claim, classifyProcessingError(err), videoObjectKey, "")
	}

	outputMeta.VideoObjectKey = videoObjectKey
	outputMeta.ThumbnailObjectKey = thumbnailObjectKey
	if err := u.videos.CompleteProcessing(ctx, repository.ProcessingCompletionInput{
		JobID:      claim.Job.ID,
		OutputMeta: outputMeta,
		Now:        time.Now(),
	}); err != nil {
		cleanupErr := u.createRollbackCleanup(ctx, claim.Video.ID, videoObjectKey, thumbnailObjectKey, time.Now())
		if cleanupErr != nil {
			return true, errors.Join(err, cleanupErr)
		}
		return true, err
	}

	return true, nil
}

func (u *videoProcessingUsecase) ExpireUploads(ctx context.Context, now time.Time, limit int) (int, error) {
	if now.IsZero() || limit < 1 {
		return 0, entity.ErrInvalidInput
	}
	return u.videos.ExpireUploads(ctx, now, limit)
}

func (u *videoProcessingUsecase) RecoverTimedOutJobs(ctx context.Context, now time.Time, limit int) (int, error) {
	if now.IsZero() || limit < 1 {
		return 0, entity.ErrInvalidInput
	}
	now = now
	return u.jobs.RecoverTimedOut(ctx, repository.ProcessingRecoveryInput{
		TimeoutBefore: now.Add(-videoProcessingTimeout),
		Now:           now,
		Limit:         limit,
		RetryDelays:   u.retryDelays,
	})
}

func (u *videoProcessingUsecase) DeleteExpiredIdempotencyRecords(ctx context.Context, now time.Time, limit int) (int64, error) {
	if now.IsZero() || limit < 1 {
		return 0, entity.ErrInvalidInput
	}
	return u.videos.DeleteExpiredIdempotencyRecords(ctx, now, limit)
}

func (u *videoProcessingUsecase) recordFailure(ctx context.Context, claim *repository.ProcessingClaim, failure processingFailure, generatedVideoKey, generatedThumbnailKey string) error {
	if claim == nil || claim.Job == nil || claim.Video == nil || claim.Job.ID == 0 || claim.Video.ID == 0 {
		return entity.ErrProcessingJobConflict
	}

	now := time.Now()
	message := normalizeProcessingFailureMessage(failure.Message)

	if failure.Retryable && claim.Job.AttemptCount < claim.Job.MaxAttempts {
		cleanupErr := u.createRollbackCleanup(ctx, claim.Video.ID, generatedVideoKey, generatedThumbnailKey, now)

		retryIndex := claim.Job.AttemptCount - 1
		if retryIndex < 0 || retryIndex >= len(u.retryDelays) {
			return errors.Join(cleanupErr, entity.ErrProcessingJobConflict)
		}
		retryErr := u.jobs.ScheduleRetry(ctx, claim.Job.ID, now.Add(u.retryDelays[retryIndex]), string(failure.Code), message, now)
		if cleanupErr != nil || retryErr != nil {
			return errors.Join(cleanupErr, retryErr)
		}
		return nil
	}

	return u.videos.FailProcessing(ctx, repository.ProcessingFailureInput{
		JobID:                 claim.Job.ID,
		FailureCode:           failure.Code,
		FailureMessage:        message,
		GeneratedVideoKey:     generatedVideoKey,
		GeneratedThumbnailKey: generatedThumbnailKey,
		Now:                   now,
	})
}

func (u *videoProcessingUsecase) createRollbackCleanup(ctx context.Context, videoID uint64, videoObjectKey, thumbnailObjectKey string, now time.Time) error {
	var joined error

	if strings.TrimSpace(videoObjectKey) != "" {
		job, err := entity.NewStorageCleanupJob(&videoID, videoObjectKey, entity.StorageAssetProcessed, entity.StorageCleanupRollback, now)
		if err != nil {
			joined = errors.Join(joined, err)
		} else if err := u.cleanupJobs.Create(ctx, job); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	if strings.TrimSpace(thumbnailObjectKey) != "" {
		job, err := entity.NewStorageCleanupJob(&videoID, thumbnailObjectKey, entity.StorageAssetThumbnail, entity.StorageCleanupRollback, now)
		if err != nil {
			joined = errors.Join(joined, err)
		} else if err := u.cleanupJobs.Create(ctx, job); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	return joined
}

func buildProcessingObjectKey(prefix string, videoID uint64, kind, extension string) (string, error) {
	if prefix != "videos/" || videoID == 0 || (kind != "output" && kind != "thumbnail") || (extension != "mp4" && extension != "jpg") {
		return "", entity.ErrInvalidInput
	}
	if (kind == "output" && extension != "mp4") || (kind == "thumbnail" && extension != "jpg") {
		return "", entity.ErrInvalidInput
	}

	randomID, err := generateRandomHex(processingObjectRandomSize)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%d/%s/%s.%s", prefix, videoID, kind, randomID, extension), nil
}

func classifyProcessingError(err error) processingFailure {
	var mediaErr *repository.MediaError
	if errors.As(err, &mediaErr) {
		return processingFailure{
			Code:      mediaErr.Code,
			Retryable: mediaErr.Retryable,
			Message:   mediaErr.Message,
		}
	}

	switch {
	case errors.Is(err, entity.ErrStorageUnavailable):
		return processingFailure{
			Code:      entity.VideoFailureStorageUnavailable,
			Retryable: true,
			Message:   "object storage is temporarily unavailable",
		}
	case errors.Is(err, entity.ErrObjectNotFound):
		return processingFailure{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "source video object was not found",
		}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return processingFailure{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: true,
			Message:   "video processing timed out",
		}
	default:
		return processingFailure{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: true,
			Message:   "video processing failed",
		}
	}
}

func normalizeProcessingFailureMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "video processing failed"
	}

	runes := []rune(message)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return message
}
