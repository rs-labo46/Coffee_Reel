package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type videoProcessingUsecaseMock struct {
	processNextFunc                     func(context.Context) (bool, error)
	expireUploadsFunc                   func(context.Context, time.Time, int) (int, error)
	recoverTimedOutJobsFunc             func(context.Context, time.Time, int) (int, error)
	deleteExpiredIdempotencyRecordsFunc func(context.Context, time.Time, int) (int64, error)
}

func (m *videoProcessingUsecaseMock) ProcessNext(
	ctx context.Context,
) (bool, error) {
	if m.processNextFunc == nil {
		return false, nil
	}

	return m.processNextFunc(ctx)
}

func (m *videoProcessingUsecaseMock) ExpireUploads(
	ctx context.Context,
	now time.Time,
	limit int,
) (int, error) {
	if m.expireUploadsFunc == nil {
		return 0, nil
	}

	return m.expireUploadsFunc(ctx, now, limit)
}

func (m *videoProcessingUsecaseMock) RecoverTimedOutJobs(
	ctx context.Context,
	now time.Time,
	limit int,
) (int, error) {
	if m.recoverTimedOutJobsFunc == nil {
		return 0, nil
	}

	return m.recoverTimedOutJobsFunc(ctx, now, limit)
}

func (m *videoProcessingUsecaseMock) DeleteExpiredIdempotencyRecords(
	ctx context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	if m.deleteExpiredIdempotencyRecordsFunc == nil {
		return 0, nil
	}

	return m.deleteExpiredIdempotencyRecordsFunc(
		ctx,
		now,
		limit,
	)
}

type storageCleanupUsecaseMock struct {
	processNextFunc         func(context.Context) (bool, error)
	recoverTimedOutJobsFunc func(context.Context, time.Time, int) (int, error)
	detectOrphanObjectsFunc func(context.Context, time.Time) (int, error)
}

func (m *storageCleanupUsecaseMock) ProcessNext(
	ctx context.Context,
) (bool, error) {
	if m.processNextFunc == nil {
		return false, nil
	}

	return m.processNextFunc(ctx)
}

func (m *storageCleanupUsecaseMock) RecoverTimedOutJobs(
	ctx context.Context,
	now time.Time,
	limit int,
) (int, error) {
	if m.recoverTimedOutJobsFunc == nil {
		return 0, nil
	}

	return m.recoverTimedOutJobsFunc(ctx, now, limit)
}

func (m *storageCleanupUsecaseMock) DetectOrphanObjects(
	ctx context.Context,
	now time.Time,
) (int, error) {
	if m.detectOrphanObjectsFunc == nil {
		return 0, nil
	}

	return m.detectOrphanObjectsFunc(ctx, now)
}

type timedLimitCall struct {
	now   time.Time
	limit int
}

func validWorkerConfig() Config {
	return Config{
		ProcessingConcurrency:      2,
		ProcessingPollInterval:     time.Second,
		CleanupPollInterval:        time.Second,
		UploadExpiryInterval:       time.Second,
		UploadExpiryLimit:          20,
		ProcessingRecoveryInterval: time.Second,
		ProcessingRecoveryLimit:    20,
		CleanupRecoveryInterval:    time.Second,
		CleanupRecoveryLimit:       20,
		OrphanDetectionInterval:    time.Second,
		IdempotencyCleanupInterval: time.Second,
		IdempotencyCleanupLimit:    100,
	}
}

func newTestLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func receiveSignal(
	t *testing.T,
	calls <-chan struct{},
	name string,
) {
	t.Helper()

	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatalf("%s was not called", name)
	}
}

func receiveTimedLimitCall(
	t *testing.T,
	calls <-chan timedLimitCall,
	name string,
) timedLimitCall {
	t.Helper()

	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatalf("%s was not called", name)
		return timedLimitCall{}
	}
}

func receiveTimeCall(
	t *testing.T,
	calls <-chan time.Time,
	name string,
) time.Time {
	t.Helper()

	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatalf("%s was not called", name)
		return time.Time{}
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func TestNew(t *testing.T) {
	processing := &videoProcessingUsecaseMock{}
	cleanup := &storageCleanupUsecaseMock{}
	config := validWorkerConfig()
	logger := newTestLogger()

	worker, err := New(processing, cleanup, config, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if worker == nil {
		t.Fatal("New() worker is nil")
	}

	t.Run("missing processing", func(t *testing.T) {
		got, err := New(nil, cleanup, config, logger)
		if err == nil {
			t.Fatal("New() error is nil")
		}
		if got != nil {
			t.Fatalf("New() worker = %#v, want nil", got)
		}
	})

	t.Run("missing cleanup", func(t *testing.T) {
		got, err := New(processing, nil, config, logger)
		if err == nil {
			t.Fatal("New() error is nil")
		}
		if got != nil {
			t.Fatalf("New() worker = %#v, want nil", got)
		}
	})

	t.Run("missing logger", func(t *testing.T) {
		got, err := New(processing, cleanup, config, nil)
		if err == nil {
			t.Fatal("New() error is nil")
		}
		if got != nil {
			t.Fatalf("New() worker = %#v, want nil", got)
		}
	})

	t.Run("invalid configuration", func(t *testing.T) {
		tests := []struct {
			name   string
			change func(*Config)
		}{
			{
				name: "processing concurrency",
				change: func(config *Config) {
					config.ProcessingConcurrency = 0
				},
			},
			{
				name: "processing poll interval",
				change: func(config *Config) {
					config.ProcessingPollInterval = 0
				},
			},
			{
				name: "cleanup poll interval",
				change: func(config *Config) {
					config.CleanupPollInterval = 0
				},
			},
			{
				name: "upload expiry interval",
				change: func(config *Config) {
					config.UploadExpiryInterval = 0
				},
			},
			{
				name: "upload expiry limit",
				change: func(config *Config) {
					config.UploadExpiryLimit = 0
				},
			},
			{
				name: "processing recovery interval",
				change: func(config *Config) {
					config.ProcessingRecoveryInterval = 0
				},
			},
			{
				name: "processing recovery limit",
				change: func(config *Config) {
					config.ProcessingRecoveryLimit = 0
				},
			},
			{
				name: "cleanup recovery interval",
				change: func(config *Config) {
					config.CleanupRecoveryInterval = 0
				},
			},
			{
				name: "cleanup recovery limit",
				change: func(config *Config) {
					config.CleanupRecoveryLimit = 0
				},
			},
			{
				name: "orphan detection interval",
				change: func(config *Config) {
					config.OrphanDetectionInterval = 0
				},
			},
			{
				name: "idempotency cleanup interval",
				change: func(config *Config) {
					config.IdempotencyCleanupInterval = 0
				},
			},
			{
				name: "idempotency cleanup limit",
				change: func(config *Config) {
					config.IdempotencyCleanupLimit = 0
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				invalid := validWorkerConfig()
				tt.change(&invalid)

				got, err := New(
					processing,
					cleanup,
					invalid,
					logger,
				)
				if err == nil {
					t.Fatal("New() error is nil")
				}
				if got != nil {
					t.Fatalf("New() worker = %#v, want nil", got)
				}
			})
		}
	})
}

func TestRunStartsAllTasksAndStops(t *testing.T) {
	processingCalls := make(chan struct{}, 2)
	cleanupCalls := make(chan struct{}, 1)
	expiryCalls := make(chan timedLimitCall, 1)
	processingRecoveryCalls := make(chan timedLimitCall, 1)
	cleanupRecoveryCalls := make(chan timedLimitCall, 1)
	orphanCalls := make(chan time.Time, 1)
	idempotencyCalls := make(chan timedLimitCall, 1)

	processing := &videoProcessingUsecaseMock{
		processNextFunc: func(context.Context) (bool, error) {
			processingCalls <- struct{}{}
			return false, nil
		},
		expireUploadsFunc: func(
			_ context.Context,
			now time.Time,
			limit int,
		) (int, error) {
			expiryCalls <- timedLimitCall{now: now, limit: limit}
			return 0, nil
		},
		recoverTimedOutJobsFunc: func(
			_ context.Context,
			now time.Time,
			limit int,
		) (int, error) {
			processingRecoveryCalls <- timedLimitCall{
				now:   now,
				limit: limit,
			}
			return 0, nil
		},
		deleteExpiredIdempotencyRecordsFunc: func(
			_ context.Context,
			now time.Time,
			limit int,
		) (int64, error) {
			idempotencyCalls <- timedLimitCall{
				now:   now,
				limit: limit,
			}
			return 0, nil
		},
	}

	cleanup := &storageCleanupUsecaseMock{
		processNextFunc: func(context.Context) (bool, error) {
			cleanupCalls <- struct{}{}
			return false, nil
		},
		recoverTimedOutJobsFunc: func(
			_ context.Context,
			now time.Time,
			limit int,
		) (int, error) {
			cleanupRecoveryCalls <- timedLimitCall{
				now:   now,
				limit: limit,
			}
			return 0, nil
		},
		detectOrphanObjectsFunc: func(
			_ context.Context,
			now time.Time,
		) (int, error) {
			orphanCalls <- now
			return 0, nil
		},
	}

	config := validWorkerConfig()
	config.ProcessingPollInterval = time.Hour
	config.CleanupPollInterval = time.Hour
	config.UploadExpiryInterval = time.Hour
	config.ProcessingRecoveryInterval = time.Hour
	config.CleanupRecoveryInterval = time.Hour
	config.OrphanDetectionInterval = time.Hour
	config.IdempotencyCleanupInterval = time.Hour

	worker, err := New(
		processing,
		cleanup,
		config,
		newTestLogger(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	for index := 0; index < config.ProcessingConcurrency; index++ {
		receiveSignal(t, processingCalls, "ProcessNext")
	}
	receiveSignal(t, cleanupCalls, "cleanup ProcessNext")

	expiry := receiveTimedLimitCall(t, expiryCalls, "ExpireUploads")
	processingRecovery := receiveTimedLimitCall(
		t,
		processingRecoveryCalls,
		"processing RecoverTimedOutJobs",
	)
	cleanupRecovery := receiveTimedLimitCall(
		t,
		cleanupRecoveryCalls,
		"cleanup RecoverTimedOutJobs",
	)
	orphanNow := receiveTimeCall(
		t,
		orphanCalls,
		"DetectOrphanObjects",
	)
	idempotency := receiveTimedLimitCall(
		t,
		idempotencyCalls,
		"DeleteExpiredIdempotencyRecords",
	)

	cancel()
	waitDone(t, done)

	assertCurrentTime := func(name string, now time.Time) {
		t.Helper()
		if now.IsZero() {
			t.Fatalf("%s now is zero", name)
		}
	}

	assertCurrentTime("ExpireUploads", expiry.now)
	assertCurrentTime("processing recovery", processingRecovery.now)
	assertCurrentTime("cleanup recovery", cleanupRecovery.now)
	assertCurrentTime("orphan detection", orphanNow)
	assertCurrentTime("idempotency cleanup", idempotency.now)

	if expiry.limit != config.UploadExpiryLimit {
		t.Fatalf(
			"ExpireUploads limit = %d, want %d",
			expiry.limit,
			config.UploadExpiryLimit,
		)
	}
	if processingRecovery.limit != config.ProcessingRecoveryLimit {
		t.Fatalf(
			"processing recovery limit = %d, want %d",
			processingRecovery.limit,
			config.ProcessingRecoveryLimit,
		)
	}
	if cleanupRecovery.limit != config.CleanupRecoveryLimit {
		t.Fatalf(
			"cleanup recovery limit = %d, want %d",
			cleanupRecovery.limit,
			config.CleanupRecoveryLimit,
		)
	}
	if idempotency.limit != config.IdempotencyCleanupLimit {
		t.Fatalf(
			"idempotency cleanup limit = %d, want %d",
			idempotency.limit,
			config.IdempotencyCleanupLimit,
		)
	}
}

func TestRunQueueContinuesImmediatelyAfterProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	worker := &Worker{logger: newTestLogger()}

	worker.runQueue(
		ctx,
		"video_processing",
		time.Hour,
		func(context.Context) (bool, error) {
			call := calls.Add(1)
			if call == 1 {
				return true, nil
			}

			cancel()
			return false, nil
		},
	)

	if got := calls.Load(); got != 2 {
		t.Fatalf("process calls = %d, want 2", got)
	}
}

func TestRunQueueLogsGenericError(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	worker := &Worker{logger: logger}
	ctx, cancel := context.WithCancel(context.Background())
	secretErr := errors.New("storage credential and object key leaked")

	worker.runQueue(
		ctx,
		"storage_cleanup",
		time.Hour,
		func(context.Context) (bool, error) {
			cancel()
			return false, secretErr
		},
	)

	logged := output.String()
	if !strings.Contains(
		logged,
		"worker task failed: task=storage_cleanup",
	) {
		t.Fatalf("log = %q, want task name", logged)
	}
	if strings.Contains(logged, secretErr.Error()) {
		t.Fatalf("internal error leaked to log: %q", logged)
	}
}

func TestRunPeriodicContinuesAfterError(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	worker := &Worker{logger: logger}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	secretErr := errors.New("database constraint and stack trace")
	done := make(chan struct{})

	go func() {
		worker.runPeriodic(
			ctx,
			"upload_expiry",
			time.Millisecond,
			func(context.Context) error {
				if calls.Add(1) == 1 {
					return secretErr
				}

				cancel()
				return nil
			},
		)
		close(done)
	}()

	waitDone(t, done)

	if got := calls.Load(); got < 2 {
		t.Fatalf("periodic calls = %d, want at least 2", got)
	}

	logged := output.String()
	if !strings.Contains(
		logged,
		"worker task failed: task=upload_expiry",
	) {
		t.Fatalf("log = %q, want task name", logged)
	}
	if strings.Contains(logged, secretErr.Error()) {
		t.Fatalf("internal error leaked to log: %q", logged)
	}
}

func TestWait(t *testing.T) {
	t.Run("interval elapsed", func(t *testing.T) {
		if !wait(context.Background(), time.Millisecond) {
			t.Fatal("wait() = false, want true")
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if wait(ctx, time.Hour) {
			t.Fatal("wait() = true, want false")
		}
	})
}
