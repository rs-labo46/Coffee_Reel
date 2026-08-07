package worker

import (
	"coffee-reel/usecase"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type Config struct {
	ProcessingConcurrency      int
	ProcessingPollInterval     time.Duration
	CleanupPollInterval        time.Duration
	UploadExpiryInterval       time.Duration
	UploadExpiryLimit          int
	ProcessingRecoveryInterval time.Duration
	ProcessingRecoveryLimit    int
	CleanupRecoveryInterval    time.Duration
	CleanupRecoveryLimit       int
	OrphanDetectionInterval    time.Duration
	IdempotencyCleanupInterval time.Duration
	IdempotencyCleanupLimit    int
}

type Worker struct {
	processing usecase.IVideoProcessingUsecase
	cleanup    usecase.IStorageCleanupUsecase
	config     Config
	logger     *log.Logger
}

func New(
	processing usecase.IVideoProcessingUsecase,
	cleanup usecase.IStorageCleanupUsecase,
	config Config,
	logger *log.Logger,
) (*Worker, error) {
	if processing == nil || cleanup == nil || logger == nil {
		return nil, fmt.Errorf("worker dependency is required")
	}
	if config.ProcessingConcurrency < 1 ||
		config.ProcessingPollInterval <= 0 ||
		config.CleanupPollInterval <= 0 ||
		config.UploadExpiryInterval <= 0 ||
		config.UploadExpiryLimit < 1 ||
		config.ProcessingRecoveryInterval <= 0 ||
		config.ProcessingRecoveryLimit < 1 ||
		config.CleanupRecoveryInterval <= 0 ||
		config.CleanupRecoveryLimit < 1 ||
		config.OrphanDetectionInterval <= 0 ||
		config.IdempotencyCleanupInterval <= 0 ||
		config.IdempotencyCleanupLimit < 1 {
		return nil, fmt.Errorf("worker configuration is invalid")
	}

	return &Worker{
		processing: processing,
		cleanup:    cleanup,
		config:     config,
		logger:     logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) {
	var group sync.WaitGroup

	for index := 0; index < w.config.ProcessingConcurrency; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			w.runQueue(ctx, "video_processing", w.config.ProcessingPollInterval, w.processing.ProcessNext)
		}()
	}

	group.Add(1)
	go func() {
		defer group.Done()
		w.runQueue(ctx, "storage_cleanup", w.config.CleanupPollInterval, w.cleanup.ProcessNext)
	}()

	group.Add(1)
	go func() {
		defer group.Done()
		w.runPeriodic(ctx, "upload_expiry", w.config.UploadExpiryInterval, func(ctx context.Context) error {
			_, err := w.processing.ExpireUploads(ctx, time.Now(), w.config.UploadExpiryLimit)
			return err
		})
	}()

	group.Add(1)
	go func() {
		defer group.Done()
		w.runPeriodic(ctx, "processing_recovery", w.config.ProcessingRecoveryInterval, func(ctx context.Context) error {
			_, err := w.processing.RecoverTimedOutJobs(ctx, time.Now(), w.config.ProcessingRecoveryLimit)
			return err
		})
	}()

	group.Add(1)
	go func() {
		defer group.Done()
		w.runPeriodic(ctx, "cleanup_recovery", w.config.CleanupRecoveryInterval, func(ctx context.Context) error {
			_, err := w.cleanup.RecoverTimedOutJobs(ctx, time.Now(), w.config.CleanupRecoveryLimit)
			return err
		})
	}()

	group.Add(1)
	go func() {
		defer group.Done()
		w.runPeriodic(ctx, "orphan_detection", w.config.OrphanDetectionInterval, func(ctx context.Context) error {
			_, err := w.cleanup.DetectOrphanObjects(ctx, time.Now())
			return err
		})
	}()

	group.Add(1)
	go func() {
		defer group.Done()
		w.runPeriodic(ctx, "idempotency_cleanup", w.config.IdempotencyCleanupInterval, func(ctx context.Context) error {
			_, err := w.processing.DeleteExpiredIdempotencyRecords(ctx, time.Now(), w.config.IdempotencyCleanupLimit)
			return err
		})
	}()

	group.Wait()
}

func (w *Worker) runQueue(
	ctx context.Context,
	name string,
	interval time.Duration,
	process func(context.Context) (bool, error),
) {
	for {
		if ctx.Err() != nil {
			return
		}

		processed, err := process(ctx)
		if err != nil {
			w.logFailure(name)
			if !wait(ctx, interval) {
				return
			}
			continue
		}
		if processed {
			continue
		}
		if !wait(ctx, interval) {
			return
		}
	}
}

func (w *Worker) runPeriodic(
	ctx context.Context,
	name string,
	interval time.Duration,
	run func(context.Context) error,
) {
	if err := run(ctx); err != nil && ctx.Err() == nil {
		w.logFailure(name)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := run(ctx); err != nil && ctx.Err() == nil {
				w.logFailure(name)
			}
		}
	}
}

func (w *Worker) logFailure(task string) {
	w.logger.Printf("worker task failed: task=%s", task)
}

func wait(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
