package model

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"coffee-reel/entity"
)

const secretMinimumBytes = 32

func NewConfig() (entity.Config, error) {
	environment, err := readEnvironment()
	if err != nil {
		return entity.Config{}, err
	}

	port, err := readPort("PORT", "8080")
	if err != nil {
		return entity.Config{}, err
	}

	redisPort, err := readPort("REDIS_PORT", "")
	if err != nil {
		return entity.Config{}, err
	}

	redisDB, err := readInt("REDIS_DB", 0)
	if err != nil {
		return entity.Config{}, err
	}

	cookieSecure, err := readBool("COOKIE_SECURE")
	if err != nil {
		return entity.Config{}, err
	}

	forcePathStyle, err := readBool("STORAGE_FORCE_PATH_STYLE")
	if err != nil {
		return entity.Config{}, err
	}

	uploadURLTTL, err := readDuration("STORAGE_UPLOAD_URL_TTL")
	if err != nil {
		return entity.Config{}, err
	}

	readURLTTL, err := readDuration("STORAGE_READ_URL_TTL")
	if err != nil {
		return entity.Config{}, err
	}

	idempotencyTTL, err := readDuration("VIDEO_IDEMPOTENCY_TTL")
	if err != nil {
		return entity.Config{}, err
	}

	idempotencyKeyMaxBytes, err := readInt("VIDEO_IDEMPOTENCY_KEY_MAX_BYTES", 1)
	if err != nil {
		return entity.Config{}, err
	}

	provider, err := requiredValue("STORAGE_PROVIDER")
	if err != nil {
		return entity.Config{}, err
	}
	if strings.ToLower(provider) != "s3" {
		return entity.Config{}, fmt.Errorf("STORAGE_PROVIDER must be s3")
	}

	databaseURL, err := requiredValue("DATABASE_URL")
	if err != nil {
		return entity.Config{}, err
	}
	redisHost, err := requiredValue("REDIS_HOST")
	if err != nil {
		return entity.Config{}, err
	}
	frontendURL, err := requiredValue("FE_URL")
	if err != nil {
		return entity.Config{}, err
	}
	jwtSecret, err := requiredSecret("JWT_SECRET")
	if err != nil {
		return entity.Config{}, err
	}
	refreshTokenHMACKey, err := requiredSecret("REFRESH_TOKEN_HMAC_KEY")
	if err != nil {
		return entity.Config{}, err
	}
	rateLimitHMACKey, err := requiredSecret("RATE_LIMIT_HMAC_KEY")
	if err != nil {
		return entity.Config{}, err
	}
	storageEndpoint, err := requiredValue("STORAGE_ENDPOINT")
	if err != nil {
		return entity.Config{}, err
	}
	storageRegion, err := requiredValue("STORAGE_REGION")
	if err != nil {
		return entity.Config{}, err
	}
	storageBucket, err := requiredValue("STORAGE_BUCKET")
	if err != nil {
		return entity.Config{}, err
	}
	storageAccessKeyID, err := requiredValue("STORAGE_ACCESS_KEY_ID")
	if err != nil {
		return entity.Config{}, err
	}
	storageSecretAccessKey, err := requiredSecretValue("STORAGE_SECRET_ACCESS_KEY")
	if err != nil {
		return entity.Config{}, err
	}
	storageManagedPrefix, err := requiredValue("STORAGE_MANAGED_PREFIX")
	if err != nil {
		return entity.Config{}, err
	}
	idempotencyHMACKey, err := requiredSecret("VIDEO_IDEMPOTENCY_HMAC_KEY")
	if err != nil {
		return entity.Config{}, err
	}

	return entity.Config{
		Environment: environment,
		Port:        port,
		DatabaseURL: databaseURL,
		Redis: entity.RedisConfig{
			Host:     redisHost,
			Port:     redisPort,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       redisDB,
		},
		FrontendURL:         frontendURL,
		CookieSecure:        cookieSecure,
		CookieDomain:        strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),
		JWTSecret:           jwtSecret,
		RefreshTokenHMACKey: refreshTokenHMACKey,
		RateLimitHMACKey:    rateLimitHMACKey,
		Storage: entity.ObjectStorageConfig{
			Endpoint:        storageEndpoint,
			PresignEndpoint: strings.TrimSpace(os.Getenv("STORAGE_PRESIGN_ENDPOINT")),
			Region:          storageRegion,
			Bucket:          storageBucket,
			AccessKeyID:     storageAccessKeyID,
			SecretAccessKey: storageSecretAccessKey,
			ManagedPrefix:   storageManagedPrefix,
			ForcePathStyle:  forcePathStyle,
			RequireHTTPS:    environment.IsProduction(),
		},
		StorageUploadURLTTL:         uploadURLTTL,
		StorageReadURLTTL:           readURLTTL,
		VideoIdempotencyTTL:         idempotencyTTL,
		VideoIdempotencyHMACKey:     idempotencyHMACKey,
		VideoIdempotencyKeyMaxBytes: idempotencyKeyMaxBytes,
	}, nil
}

func readEnvironment() (entity.Environment, error) {
	value, err := requiredValue("ENVIRONMENT")
	if err != nil {
		return "", err
	}

	environment := entity.Environment(strings.ToLower(value))
	if !environment.IsValid() {
		return "", fmt.Errorf("ENVIRONMENT must be develop or production")
	}

	return environment, nil
}

func readPort(name, defaultValue string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		value = defaultValue
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return "", fmt.Errorf("%s must be between 1 and 65535", name)
	}

	return value, nil
}

func readDuration(name string) (time.Duration, error) {
	value, err := requiredValue(name)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}

	return duration, nil
}

func readInt(name string, minimum int) (int, error) {
	value, err := requiredValue(name)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum {
		return 0, fmt.Errorf("%s must be an integer greater than or equal to %d", name, minimum)
	}

	return parsed, nil
}

func readBool(name string) (bool, error) {
	value, err := requiredValue(name)
	if err != nil {
		return false, err
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}

	return parsed, nil
}

func requiredValue(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return value, nil
}

func requiredSecret(name string) (string, error) {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" || len([]byte(value)) < secretMinimumBytes {
		return "", fmt.Errorf("%s must be at least 32 bytes", name)
	}

	return value, nil
}

func requiredSecretValue(name string) (string, error) {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return value, nil
}
