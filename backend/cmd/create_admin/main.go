package main

import (
	"coffee-reel/db"
	"coffee-reel/repository"
	"coffee-reel/usecase"
	"coffee-reel/validator"
	"context"
	"log"
	"os"
	"strings"
)

const (
	adminNameEnv     = "ADMIN_NAME"
	adminEmailEnv    = "ADMIN_EMAIL"
	adminPasswordEnv = "ADMIN_PASSWORD"
)

func main() {
	name := os.Getenv(adminNameEnv)
	email := os.Getenv(adminEmailEnv)
	password := os.Getenv(adminPasswordEnv)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" || password == "" {
		log.Fatal("ADMIN_NAME, ADMIN_EMAIL and ADMIN_PASSWORD are required")
	}

	postgresDB, err := db.NewDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println(err)
		}
	}()

	tokenService, err := usecase.NewTokenService(
		os.Getenv("JWT_SECRET"),
		os.Getenv("REFRESH_TOKEN_HMAC_KEY"),
	)
	if err != nil {
		log.Fatal(err)
	}

	userRepository := repository.NewUserRepository(postgresDB)
	adminUserRepository := repository.NewAdminUserRepository(postgresDB)
	userValidator := validator.NewUserValidator()
	adminValidator := validator.NewAdminUserValidator(userValidator)
	adminUsecase := usecase.NewAdminUserUsecase(userRepository, adminUserRepository, tokenService)

	validatedName, validatedEmail, validatedPassword, err := adminValidator.ValidateCreateAdmin(name, email, password)
	if err != nil {
		log.Fatal("admin input is invalid")
	}

	_, created, err := adminUsecase.CreateAdmin(
		context.Background(),
		validatedName,
		validatedEmail,
		validatedPassword,
	)
	if err != nil {
		log.Fatal("create admin failed")
	}

	if created {
		log.Println("admin created")
		return
	}
	log.Println("admin already exists")
}
