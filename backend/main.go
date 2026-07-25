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
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	postgresDB, err := db.NewDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println(err)
		}
	}()

	redisDB := 0
	if value := os.Getenv("REDIS_DB"); value != "" {
		redisDB, err = strconv.Atoi(value)
		if err != nil {
			log.Fatal("REDIS_DB must be an integer")
		}
	}

	redisClient := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})

	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Println(err)
		}
	}()

	redisContext, redisCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer redisCancel()

	if err := redisClient.Ping(redisContext).Err(); err != nil {
		log.Fatal(err)
	}

	userRepository := repository.NewUserRepository(postgresDB)
	refreshTokenRepository := repository.NewRefreshTokenRepository(postgresDB)
	rateLimitRepository := repository.NewRateLimitRepository(redisClient)
	adminUserRepository := repository.NewAdminUserRepository(postgresDB)

	tokenService, err := usecase.NewTokenService(os.Getenv("JWT_SECRET"), os.Getenv("REFRESH_TOKEN_HMAC_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	userUsecase := usecase.NewUserUsecase(userRepository, refreshTokenRepository, tokenService)
	adminUserUsecase := usecase.NewAdminUserUsecase(userRepository, adminUserRepository, tokenService)

	rateLimitUsecase, err := usecase.NewRateLimitUsecase(rateLimitRepository, tokenService, os.Getenv("RATE_LIMIT_HMAC_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	userValidator := validator.NewUserValidator()
	adminUserValidator := validator.NewAdminUserValidator(userValidator)

	cookieSecure := false
	if value := os.Getenv("COOKIE_SECURE"); value != "" {
		cookieSecure, err = strconv.ParseBool(value)
		if err != nil {
			log.Fatal("COOKIE_SECURE must be true or false")
		}
	}

	userController := controller.NewUserController(userUsecase, rateLimitUsecase, userValidator, controller.CookieConfig{Secure: cookieSecure, CSRFDomain: os.Getenv("COOKIE_DOMAIN")})

	authMiddleware := middleware.NewAuthMiddleware(userUsecase, tokenService)
	csrfMiddleware := middleware.NewCSRFMiddleware(tokenService)
	adminMiddleware := middleware.NewAdminMiddleware()
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimitUsecase)

	e := router.NewRouter(userController, authMiddleware, csrfMiddleware, rateLimitMiddleware, os.Getenv("FE_URL"))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
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
			log.Println(err)
		}

	case <-ctx.Done():
		shutdownContext, shutdownCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer shutdownCancel()

		if err := e.Shutdown(shutdownContext); err != nil {
			log.Println(err)
		}
	}
}
