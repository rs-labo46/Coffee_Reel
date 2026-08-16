package main

import (
	"coffee-reel/controller"
	"coffee-reel/db"
	"coffee-reel/middleware"
	"coffee-reel/model"
	"coffee-reel/repository"
	"coffee-reel/router"
	"coffee-reel/usecase"
	"coffee-reel/validator"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config, err := model.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	postgresDB, err := db.NewDB(config.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println("close database failed")
		}
	}()

	redisClient, err := model.NewRedis(ctx, config.Redis)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Println("close redis failed")
		}
	}()

	storageContext, storageCancel := context.WithTimeout(ctx, 10*time.Second)
	defer storageCancel()
	storageRepository, err := repository.NewObjectStorageRepository(storageContext, config.Storage)
	if err != nil {
		log.Fatal(err)
	}

	healthRepository := repository.NewHealthRepository(postgresDB, redisClient, storageRepository)

	userRepository := repository.NewUserRepository(postgresDB)
	refreshTokenRepository := repository.NewRefreshTokenRepository(postgresDB)
	rateLimitRepository := repository.NewRateLimitRepository(redisClient)
	adminUserRepository := repository.NewAdminUserRepository(postgresDB)
	adminVideoRepository := repository.NewAdminVideoRepository(postgresDB)
	videoRepository := repository.NewVideoRepository(postgresDB)
	videoLikeRepository := repository.NewVideoLikeRepository(postgresDB)
	savedVideoRepository := repository.NewSavedVideoRepository(postgresDB)

	tokenService, err := usecase.NewTokenService(config.JWTSecret, config.RefreshTokenHMACKey)
	if err != nil {
		log.Fatal(err)
	}

	userUsecase := usecase.NewUserUsecase(userRepository, refreshTokenRepository, tokenService)
	adminUserUsecase := usecase.NewAdminUserUsecase(userRepository, adminUserRepository, tokenService)
	adminVideoUsecase, err := usecase.NewAdminVideoUsecase(
		adminVideoRepository,
		storageRepository,
		usecase.AdminVideoUsecaseConfig{
			ReadURLTTL: config.StorageReadURLTTL,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	rateLimitUsecase, err := usecase.NewRateLimitUsecase(rateLimitRepository, tokenService, config.RateLimitHMACKey)
	if err != nil {
		log.Fatal(err)
	}

	videoUsecase, err := usecase.NewVideoUsecase(videoRepository, storageRepository, usecase.VideoUsecaseConfig{
		UploadURLTTL:       config.StorageUploadURLTTL,
		ReadURLTTL:         config.StorageReadURLTTL,
		IdempotencyTTL:     config.VideoIdempotencyTTL,
		IdempotencyHMACKey: []byte(config.VideoIdempotencyHMACKey),
		ManagedPrefix:      config.Storage.ManagedPrefix,
	})
	if err != nil {
		log.Fatal(err)
	}

	healthUsecase := usecase.NewHealthUsecase(healthRepository)
	videoLikeUsecase := usecase.NewVideoLikeUsecase(videoLikeRepository)

	savedVideoUsecase, err := usecase.NewSavedVideoUsecase(savedVideoRepository, storageRepository, usecase.SavedVideoUsecaseConfig{
		ReadURLTTL: config.StorageReadURLTTL,
	})
	if err != nil {
		log.Fatal(err)
	}

	userValidator := validator.NewUserValidator()
	adminUserValidator := validator.NewAdminUserValidator(userValidator)
	adminVideoValidator := validator.NewAdminVideoValidator()
	videoValidator, err := validator.NewVideoValidator(validator.VideoValidatorConfig{
		IdempotencyKeyMaxBytes: config.VideoIdempotencyKeyMaxBytes,
	})
	if err != nil {
		log.Fatal(err)
	}

	userController := controller.NewUserController(
		userUsecase,
		rateLimitUsecase,
		userValidator,
		controller.CookieConfig{
			Secure:     config.CookieSecure,
			CSRFDomain: config.CookieDomain,
		},
	)
	adminUserController := controller.NewAdminUserController(adminUserUsecase, adminUserValidator)
	adminVideoController := controller.NewAdminVideoController(adminVideoUsecase, adminVideoValidator)
	videoController := controller.NewVideoController(videoUsecase, videoValidator)
	healthController := controller.NewHealthController(healthUsecase)
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
		config.FrontendURL,
		healthController,
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

	serverError := make(chan error, 1)
	go func() {
		serverError <- e.Start(":" + config.Port)
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
