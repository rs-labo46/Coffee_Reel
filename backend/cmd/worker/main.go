package main

import (
	"coffee-reel/db"
	"coffee-reel/repository"
	"coffee-reel/usecase"
	"coffee-reel/worker"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	postgresDB, err := db.NewDB(requiredEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println("close database failed")
		}
	}()

	storageContext, storageCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer storageCancel()
	storageRepository, err := repository.NewObjectStorageRepository(storageContext, objectStorageConfig())
	if err != nil {
		log.Fatal(err)
	}

	mediaRepository, err := repository.NewMediaRepository(requiredEnv("FFPROBE_PATH"), requiredEnv("FFMPEG_PATH"))
	if err != nil {
		log.Fatal(err)
	}

	videoRepository := repository.NewVideoRepository(postgresDB)
	processingJobRepository := repository.NewVideoProcessingJobRepository(postgresDB)
	cleanupJobRepository := repository.NewStorageCleanupJobRepository(postgresDB)

	processingUsecase, err := usecase.NewVideoProcessingUsecase(
		videoRepository,
		processingJobRepository,
		storageRepository,
		mediaRepository,
		cleanupJobRepository,
		usecase.VideoProcessingUsecaseConfig{
			RetryDelays:   requiredDurationListEnv("VIDEO_PROCESSING_RETRY_DELAYS"),
			ManagedPrefix: requiredEnv("STORAGE_MANAGED_PREFIX"),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	cleanupUsecase, err := usecase.NewStorageCleanupUsecase(
		cleanupJobRepository,
		storageRepository,
		videoRepository,
		usecase.StorageCleanupUsecaseConfig{
			RetryDelays:  requiredDurationListEnv("STORAGE_CLEANUP_RETRY_DELAYS"),
			Timeout:      requiredDurationEnv("STORAGE_CLEANUP_TIMEOUT"),
			OrphanMinAge: requiredDurationEnv("STORAGE_ORPHAN_MIN_AGE"),
			ListLimit:    int32(requiredIntEnv("STORAGE_ORPHAN_LIST_LIMIT", 1)),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	runner, err := worker.New(
		processingUsecase,
		cleanupUsecase,
		worker.Config{
			ProcessingConcurrency:      requiredIntEnv("WORKER_PROCESSING_CONCURRENCY", 1),
			ProcessingPollInterval:     requiredDurationEnv("WORKER_PROCESSING_POLL_INTERVAL"),
			CleanupPollInterval:        requiredDurationEnv("WORKER_CLEANUP_POLL_INTERVAL"),
			UploadExpiryInterval:       requiredDurationEnv("WORKER_UPLOAD_EXPIRY_INTERVAL"),
			UploadExpiryLimit:          requiredIntEnv("WORKER_UPLOAD_EXPIRY_LIMIT", 1),
			ProcessingRecoveryInterval: requiredDurationEnv("WORKER_PROCESSING_RECOVERY_INTERVAL"),
			ProcessingRecoveryLimit:    requiredIntEnv("WORKER_PROCESSING_RECOVERY_LIMIT", 1),
			CleanupRecoveryInterval:    requiredDurationEnv("WORKER_CLEANUP_RECOVERY_INTERVAL"),
			CleanupRecoveryLimit:       requiredIntEnv("WORKER_CLEANUP_RECOVERY_LIMIT", 1),
			OrphanDetectionInterval:    requiredDurationEnv("WORKER_ORPHAN_DETECTION_INTERVAL"),
			IdempotencyCleanupInterval: requiredDurationEnv("WORKER_IDEMPOTENCY_CLEANUP_INTERVAL"),
			IdempotencyCleanupLimit:    requiredIntEnv("WORKER_IDEMPOTENCY_CLEANUP_LIMIT", 1),
		},
		log.Default(),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner.Run(ctx)
}

func objectStorageConfig() repository.ObjectStorageConfig {
	provider := strings.ToLower(requiredEnv("STORAGE_PROVIDER"))
	if provider != "s3" {
		log.Fatal("STORAGE_PROVIDER must be s3")
	}

	environment := strings.ToLower(strings.TrimSpace(os.Getenv("GO_ENV")))
	requireHTTPS := environment == "production" || environment == "prod"

	return repository.ObjectStorageConfig{
		Endpoint:        requiredEnv("STORAGE_ENDPOINT"),
		PresignEndpoint: strings.TrimSpace(os.Getenv("STORAGE_PRESIGN_ENDPOINT")),
		Region:          requiredEnv("STORAGE_REGION"),
		Bucket:          requiredEnv("STORAGE_BUCKET"),
		AccessKeyID:     requiredEnv("STORAGE_ACCESS_KEY_ID"),
		SecretAccessKey: requiredEnv("STORAGE_SECRET_ACCESS_KEY"),
		ManagedPrefix:   requiredEnv("STORAGE_MANAGED_PREFIX"),
		ForcePathStyle:  requiredBoolEnv("STORAGE_FORCE_PATH_STYLE"),
		RequireHTTPS:    requireHTTPS,
	}
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatal(name + " is required")
	}
	return value
}

func requiredDurationEnv(name string) time.Duration {
	value := requiredEnv(name)
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		log.Fatal(name + " must be a positive duration")
	}
	return duration
}

func requiredDurationListEnv(name string) [3]time.Duration {
	parts := strings.Split(requiredEnv(name), ",")
	if len(parts) != 3 {
		log.Fatal(name + " must contain exactly three comma-separated durations")
	}

	var values [3]time.Duration
	for index, part := range parts {
		duration, err := time.ParseDuration(strings.TrimSpace(part))
		if err != nil || duration <= 0 {
			log.Fatal(name + " contains an invalid duration")
		}
		values[index] = duration
	}
	return values
}

func requiredIntEnv(name string, minimum int) int {
	value := requiredEnv(name)
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum {
		log.Fatal(fmt.Sprintf("%s must be an integer greater than or equal to %d", name, minimum))
	}
	return parsed
}

func requiredBoolEnv(name string) bool {
	value := requiredEnv(name)
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatal(name + " must be true or false")
	}
	return parsed
}
