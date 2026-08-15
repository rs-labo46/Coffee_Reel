package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	processStartedAt := time.Now()
	claimStartedAt := time.Now()
	claim, err := u.jobs.ClaimNext(ctx, time.Now())
	claimDuration := time.Since(claimStartedAt)
	if err != nil {
		return false, err
	}
	if claim == nil {
		return false, nil
	}
	if claim.Job == nil || claim.Video == nil || claim.Job.ID == 0 || claim.Video.ID == 0 || claim.Job.VideoID != claim.Video.ID {
		return true, entity.ErrProcessingJobConflict
	}

	trace := newVideoProcessingTrace(claim, processStartedAt)
	trace.logClaim(claimDuration)

	stageStartedAt := time.Now()
	workDir, err := os.MkdirTemp("", fmt.Sprintf("coffee-reel-video-%d-", claim.Job.ID))
	stageDuration := time.Since(stageStartedAt)
	if err != nil {
		failure := processingFailure{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: true,
			Message:   "temporary work directory could not be created",
		}
		return true, u.handleProcessingFailure(ctx, claim, trace, "workdir", stageDuration, failure, "", "")
	}
	trace.logStage("workdir", "completed", stageDuration)
	defer os.RemoveAll(workDir)

	processCtx, cancel := context.WithTimeout(ctx, videoProcessingTimeout)
	defer cancel()

	sourcePath := filepath.Join(workDir, "source.bin")
	outputPath := filepath.Join(workDir, "output.mp4")
	thumbnailPath := filepath.Join(workDir, "thumbnail.jpg")

	stageStartedAt = time.Now()
	err = u.storage.Download(processCtx, claim.Video.OriginalObjectKey, sourcePath)
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "download", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("download", "completed", stageDuration)

	sourceMeta := claim.SourceMeta
	if sourceMeta == nil {
		stageStartedAt = time.Now()
		probed, probeErr := u.media.Probe(processCtx, sourcePath)
		stageDuration = time.Since(stageStartedAt)
		if probeErr != nil {
			return true, u.handleProcessingFailure(ctx, claim, trace, "probe_source", stageDuration, classifyProcessingError(probeErr), "", "")
		}
		trace.logStage("probe_source", "completed", stageDuration)

		stageStartedAt = time.Now()
		recordErr := u.videos.RecordSourceValidation(ctx, claim.Video.ID, probed, time.Now())
		stageDuration = time.Since(stageStartedAt)
		if recordErr != nil {
			if errors.Is(recordErr, entity.ErrVideoNotFound) {
				trace.logStage("record_source", "cancelled", stageDuration)
				cancelStartedAt := time.Now()
				cancelErr := u.jobs.Cancel(ctx, claim.Job.ID, time.Now())
				cancelDuration := time.Since(cancelStartedAt)
				if cancelErr != nil {
					trace.logStage("cancel", "failed", cancelDuration)
					trace.logSummary("cancel_failed")
					return true, cancelErr
				}
				trace.logStage("cancel", "completed", cancelDuration)
				trace.logSummary("cancelled")
				return true, nil
			}
			return true, u.handleProcessingFailure(ctx, claim, trace, "record_source", stageDuration, classifyProcessingError(recordErr), "", "")
		}
		trace.logStage("record_source", "completed", stageDuration)
		sourceMeta = &probed
	} else {
		trace.logStage("probe_source", "skipped", 0)
		trace.logStage("record_source", "skipped", 0)
	}

	trace.logStageStarted("transcode")
	stageStartedAt = time.Now()
	err = u.media.Transcode(processCtx, sourcePath, outputPath, sourceMeta.HasAudio)
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "transcode", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("transcode", "completed", stageDuration)

	trace.logStageStarted("thumbnail")
	stageStartedAt = time.Now()
	err = u.media.GenerateThumbnail(processCtx, sourcePath, thumbnailPath)
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "thumbnail", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("thumbnail", "completed", stageDuration)

	trace.logStageStarted("probe_output")
	stageStartedAt = time.Now()
	outputMeta, err := u.media.ProbeOutput(processCtx, outputPath)
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "probe_output", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("probe_output", "completed", stageDuration)

	stageStartedAt = time.Now()
	videoObjectKey, err := buildProcessingObjectKey(u.managedPrefix, claim.Video.ID, "output", "mp4")
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "build_video_key", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("build_video_key", "completed", stageDuration)

	stageStartedAt = time.Now()
	thumbnailObjectKey, err := buildProcessingObjectKey(u.managedPrefix, claim.Video.ID, "thumbnail", "jpg")
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "build_thumbnail_key", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("build_thumbnail_key", "completed", stageDuration)

	trace.logStageStarted("upload_video")
	stageStartedAt = time.Now()
	err = u.storage.UploadProcessed(processCtx, videoObjectKey, outputPath)
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "upload_video", stageDuration, classifyProcessingError(err), "", "")
	}
	trace.logStage("upload_video", "completed", stageDuration)

	trace.logStageStarted("upload_thumbnail")
	stageStartedAt = time.Now()
	err = u.storage.UploadThumbnail(processCtx, thumbnailObjectKey, thumbnailPath)
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		return true, u.handleProcessingFailure(ctx, claim, trace, "upload_thumbnail", stageDuration, classifyProcessingError(err), videoObjectKey, "")
	}
	trace.logStage("upload_thumbnail", "completed", stageDuration)

	outputMeta.VideoObjectKey = videoObjectKey
	outputMeta.ThumbnailObjectKey = thumbnailObjectKey
	trace.logStageStarted("complete_processing")
	stageStartedAt = time.Now()
	err = u.videos.CompleteProcessing(ctx, repository.ProcessingCompletionInput{
		JobID:      claim.Job.ID,
		OutputMeta: outputMeta,
		Now:        time.Now(),
	})
	stageDuration = time.Since(stageStartedAt)
	if err != nil {
		trace.logStage("complete_processing", "failed", stageDuration)

		cleanupStartedAt := time.Now()
		cleanupErr := u.createRollbackCleanup(ctx, claim.Video.ID, videoObjectKey, thumbnailObjectKey, time.Now())
		cleanupDuration := time.Since(cleanupStartedAt)
		if cleanupErr != nil {
			trace.logStage("rollback_cleanup", "failed", cleanupDuration)
			trace.logSummary("failed")
			return true, errors.Join(err, cleanupErr)
		}
		trace.logStage("rollback_cleanup", "completed", cleanupDuration)
		trace.logSummary("failed")
		return true, err
	}
	trace.logStage("complete_processing", "completed", stageDuration)
	trace.logSummary("succeeded")

	return true, nil
}

func (u *videoProcessingUsecase) handleProcessingFailure(
	ctx context.Context,
	claim *repository.ProcessingClaim,
	trace *videoProcessingTrace,
	stage string,
	stageDuration time.Duration,
	failure processingFailure,
	generatedVideoKey string,
	generatedThumbnailKey string,
) error {
	trace.logFailure(stage, stageDuration, failure)

	outcome := "failed"
	if failure.Retryable && claim.Job.AttemptCount < claim.Job.MaxAttempts {
		outcome = "retry_scheduled"
	}

	recordStartedAt := time.Now()
	err := u.recordFailure(ctx, claim, failure, generatedVideoKey, generatedThumbnailKey)
	recordDuration := time.Since(recordStartedAt)
	if err != nil {
		trace.logFailureRecord("failure_record_error", recordDuration)
		trace.logSummary("failure_record_error")
		return err
	}

	trace.logFailureRecord(outcome, recordDuration)
	trace.logSummary(outcome)
	return nil
}

type videoProcessingTrace struct {
	jobID     uint64
	videoID   uint64
	attempt   int
	startedAt time.Time
	job       *entity.VideoProcessingJob
}

func newVideoProcessingTrace(claim *repository.ProcessingClaim, startedAt time.Time) *videoProcessingTrace {
	return &videoProcessingTrace{
		jobID:     claim.Job.ID,
		videoID:   claim.Video.ID,
		attempt:   claim.Job.AttemptCount,
		startedAt: startedAt,
		job:       claim.Job,
	}
}

func (t *videoProcessingTrace) logClaim(duration time.Duration) {
	log.Printf(
		"video_processing timing job_id=%d video_id=%d attempt=%d stage=claim status=completed duration_ms=%d elapsed_ms=%d queue_wait_ms=%d job_age_ms=%d",
		t.jobID,
		t.videoID,
		t.attempt,
		duration.Milliseconds(),
		t.elapsed().Milliseconds(),
		processingJobWait(t.job).Milliseconds(),
		processingJobAge(t.job).Milliseconds(),
	)
}

func (t *videoProcessingTrace) logStageStarted(stage string) {
	log.Printf(
		"video_processing timing job_id=%d video_id=%d attempt=%d stage=%s status=started elapsed_ms=%d",
		t.jobID,
		t.videoID,
		t.attempt,
		stage,
		t.elapsed().Milliseconds(),
	)
}

func (t *videoProcessingTrace) logStage(stage, status string, duration time.Duration) {
	log.Printf(
		"video_processing timing job_id=%d video_id=%d attempt=%d stage=%s status=%s duration_ms=%d elapsed_ms=%d",
		t.jobID,
		t.videoID,
		t.attempt,
		stage,
		status,
		duration.Milliseconds(),
		t.elapsed().Milliseconds(),
	)
}

func (t *videoProcessingTrace) logFailure(stage string, duration time.Duration, failure processingFailure) {
	log.Printf(
		"video_processing timing job_id=%d video_id=%d attempt=%d stage=%s status=failed duration_ms=%d elapsed_ms=%d failure_code=%s retryable=%t",
		t.jobID,
		t.videoID,
		t.attempt,
		stage,
		duration.Milliseconds(),
		t.elapsed().Milliseconds(),
		failure.Code,
		failure.Retryable,
	)
}

func (t *videoProcessingTrace) logFailureRecord(outcome string, duration time.Duration) {
	log.Printf(
		"video_processing timing job_id=%d video_id=%d attempt=%d stage=record_failure status=completed outcome=%s duration_ms=%d elapsed_ms=%d",
		t.jobID,
		t.videoID,
		t.attempt,
		outcome,
		duration.Milliseconds(),
		t.elapsed().Milliseconds(),
	)
}

func (t *videoProcessingTrace) logSummary(status string) {
	log.Printf(
		"video_processing summary job_id=%d video_id=%d attempt=%d status=%s total_ms=%d",
		t.jobID,
		t.videoID,
		t.attempt,
		status,
		t.elapsed().Milliseconds(),
	)
}

func (t *videoProcessingTrace) elapsed() time.Duration {
	return time.Since(t.startedAt)
}

func processingJobWait(job *entity.VideoProcessingJob) time.Duration {
	if job == nil || job.StartedAt == nil || job.AvailableAt.IsZero() {
		return 0
	}

	wait := job.StartedAt.Sub(job.AvailableAt)
	if wait < 0 {
		return 0
	}
	return wait
}

func processingJobAge(job *entity.VideoProcessingJob) time.Duration {
	if job == nil || job.StartedAt == nil || job.CreatedAt.IsZero() {
		return 0
	}

	age := job.StartedAt.Sub(job.CreatedAt)
	if age < 0 {
		return 0
	}
	return age
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
