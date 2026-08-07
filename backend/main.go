package main

import (
	"coffee-reel/controller"
	"coffee-reel/db"
	"coffee-reel/middleware"
	"coffee-reel/repository"
	"coffee-reel/router"
	"coffee-reel/usecase"
	"coffee-reel/validator"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

const secretMinimumBytes = 32

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

	redisClient := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(requiredEnv("REDIS_HOST"), requiredEnv("REDIS_PORT")),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       requiredIntEnv("REDIS_DB", 0),
	})
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Println("close redis failed")
		}
	}()

	redisContext, redisCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer redisCancel()
	if err := redisClient.Ping(redisContext).Err(); err != nil {
		log.Fatal("connect to redis failed")
	}

	storageContext, storageCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer storageCancel()
	storageRepository, err := repository.NewObjectStorageRepository(storageContext, objectStorageConfig())
	if err != nil {
		log.Fatal(err)
	}

	userRepository := repository.NewUserRepository(postgresDB)
	refreshTokenRepository := repository.NewRefreshTokenRepository(postgresDB)
	rateLimitRepository := repository.NewRateLimitRepository(redisClient)
	adminUserRepository := repository.NewAdminUserRepository(postgresDB)
	adminVideoRepository := repository.NewAdminVideoRepository(postgresDB)
	videoRepository := repository.NewVideoRepository(postgresDB)
	videoLikeRepository := repository.NewVideoLikeRepository(postgresDB)
	savedVideoRepository := repository.NewSavedVideoRepository(postgresDB)

	tokenService, err := usecase.NewTokenService(requiredSecretEnv("JWT_SECRET"), requiredSecretEnv("REFRESH_TOKEN_HMAC_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	userUsecase := usecase.NewUserUsecase(userRepository, refreshTokenRepository, tokenService)
	adminUserUsecase := usecase.NewAdminUserUsecase(userRepository, adminUserRepository, tokenService)
	adminVideoUsecase, err := usecase.NewAdminVideoUsecase(
		adminVideoRepository,
		storageRepository,
		usecase.AdminVideoUsecaseConfig{
			ReadURLTTL: requiredDurationEnv("STORAGE_READ_URL_TTL"),
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	rateLimitUsecase, err := usecase.NewRateLimitUsecase(rateLimitRepository, tokenService, requiredSecretEnv("RATE_LIMIT_HMAC_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	videoUsecase, err := usecase.NewVideoUsecase(videoRepository, storageRepository, usecase.VideoUsecaseConfig{
		UploadURLTTL:       requiredDurationEnv("STORAGE_UPLOAD_URL_TTL"),
		ReadURLTTL:         requiredDurationEnv("STORAGE_READ_URL_TTL"),
		IdempotencyTTL:     requiredDurationEnv("VIDEO_IDEMPOTENCY_TTL"),
		IdempotencyHMACKey: []byte(requiredSecretEnv("VIDEO_IDEMPOTENCY_HMAC_KEY")),
		ManagedPrefix:      requiredEnv("STORAGE_MANAGED_PREFIX"),
	})
	if err != nil {
		log.Fatal(err)
	}

	videoLikeUsecase := usecase.NewVideoLikeUsecase(videoLikeRepository)

	savedVideoUsecase, err := usecase.NewSavedVideoUsecase(savedVideoRepository, storageRepository, usecase.SavedVideoUsecaseConfig{
		ReadURLTTL: requiredDurationEnv("STORAGE_READ_URL_TTL"),
	})
	if err != nil {
		log.Fatal(err)
	}

	userValidator := validator.NewUserValidator()
	adminUserValidator := validator.NewAdminUserValidator(userValidator)
	adminVideoValidator := validator.NewAdminVideoValidator()
	videoValidator, err := validator.NewVideoValidator(validator.VideoValidatorConfig{
		IdempotencyKeyMaxBytes: requiredIntEnv("VIDEO_IDEMPOTENCY_KEY_MAX_BYTES", 1),
	})
	if err != nil {
		log.Fatal(err)
	}

	userController := controller.NewUserController(
		userUsecase,
		rateLimitUsecase,
		userValidator,
		controller.CookieConfig{
			Secure:     requiredBoolEnv("COOKIE_SECURE"),
			CSRFDomain: os.Getenv("COOKIE_DOMAIN"),
		},
	)
	adminUserController := controller.NewAdminUserController(adminUserUsecase, adminUserValidator)
	adminVideoController := controller.NewAdminVideoController(adminVideoUsecase, adminVideoValidator)
	videoController := controller.NewVideoController(videoUsecase, videoValidator)
	videoLikeController := controller.NewVideoLikeController(videoLikeUsecase, videoValidator)
	savedVideoController := controller.NewSavedVideoController(savedVideoUsecase, videoValidator)

	authMiddleware := middleware.NewAuthMiddleware(userUsecase, tokenService)
	csrfMiddleware := middleware.NewCSRFMiddleware(tokenService)
	adminMiddleware := middleware.NewAdminMiddleware()

	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimitUsecase)

	e := router.NewRouter(
		userController,
		authMiddleware,
		csrfMiddleware,
		rateLimitMiddleware,
		requiredEnv("FE_URL"),
		router.AdminComponents{
			Controller:      adminUserController,
			VideoController: adminVideoController,
			Middleware:      adminMiddleware,
		},
		router.VideoComponents{
			Controller:      videoController,
			SavedController: savedVideoController,
			LikeController:  videoLikeController,
		},
	)

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsedPort == 0 {
		log.Fatal("PORT must be between 1 and 65535")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverError := make(chan error, 1)
	go func() {
		serverError <- e.Start(":" + port)
	}()

	select {
	case err := <-serverError:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println("HTTP server stopped unexpectedly")
		}
	case <-ctx.Done():
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := e.Shutdown(shutdownContext); err != nil {
			log.Println("HTTP server shutdown failed")
		}
	}
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

func requiredSecretEnv(name string) string {
	value := os.Getenv(name)
	if len([]byte(value)) < secretMinimumBytes {
		log.Fatal(name + " must be at least 32 bytes")
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
