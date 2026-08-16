package model

import (
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestNewConfigLoadsDevelopEnvironment(t *testing.T) {
	setValidConfigEnvironment(t)

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if config.Environment != entity.EnvironmentDevelop {
		t.Fatalf("Environment = %q", config.Environment)
	}
	if config.Port != "8081" || config.Redis.Port != "6379" || config.Redis.DB != 0 {
		t.Fatal("port configuration does not match the environment")
	}
	if config.Storage.RequireHTTPS {
		t.Fatal("RequireHTTPS = true in develop")
	}
	if config.StorageUploadURLTTL != 15*time.Minute || config.VideoIdempotencyKeyMaxBytes != 128 {
		t.Fatal("duration or limit configuration does not match the environment")
	}
}

func TestNewConfigEnablesHTTPSInProduction(t *testing.T) {
	setValidConfigEnvironment(t)
	t.Setenv("ENVIRONMENT", "production")

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if !config.Storage.RequireHTTPS {
		t.Fatal("RequireHTTPS = false in production")
	}
}

func TestNewConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testing.T)
	}{
		{name: "missing environment", edit: func(t *testing.T) { t.Setenv("ENVIRONMENT", "") }},
		{name: "unsupported environment", edit: func(t *testing.T) { t.Setenv("ENVIRONMENT", "local") }},
		{name: "invalid port", edit: func(t *testing.T) { t.Setenv("PORT", "65536") }},
		{name: "negative redis db", edit: func(t *testing.T) { t.Setenv("REDIS_DB", "-1") }},
		{name: "invalid cookie secure", edit: func(t *testing.T) { t.Setenv("COOKIE_SECURE", "yes") }},
		{name: "short jwt secret", edit: func(t *testing.T) { t.Setenv("JWT_SECRET", "short") }},
		{name: "unsupported storage provider", edit: func(t *testing.T) { t.Setenv("STORAGE_PROVIDER", "gcs") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidConfigEnvironment(t)
			tt.edit(t)
			if _, err := NewConfig(); err == nil {
				t.Fatal("NewConfig() error = nil")
			}
		})
	}
}

func setValidConfigEnvironment(t *testing.T) {
	t.Helper()

	values := map[string]string{
		"ENVIRONMENT":                    "develop",
		"PORT":                           "8081",
		"DATABASE_URL":                   "host=localhost dbname=coffee_reel",
		"REDIS_HOST":                     "localhost",
		"REDIS_PORT":                     "6379",
		"REDIS_PASSWORD":                 "",
		"REDIS_DB":                       "0",
		"FE_URL":                         "http://localhost:3000",
		"COOKIE_SECURE":                  "false",
		"COOKIE_DOMAIN":                  "",
		"JWT_SECRET":                     "12345678901234567890123456789012",
		"REFRESH_TOKEN_HMAC_KEY":         "12345678901234567890123456789012",
		"RATE_LIMIT_HMAC_KEY":            "12345678901234567890123456789012",
		"STORAGE_PROVIDER":               "s3",
		"STORAGE_ENDPOINT":               "http://localhost:9000",
		"STORAGE_PRESIGN_ENDPOINT":       "http://localhost:9000",
		"STORAGE_REGION":                 "us-east-1",
		"STORAGE_BUCKET":                 "coffee-reel",
		"STORAGE_ACCESS_KEY_ID":          "access-key",
		"STORAGE_SECRET_ACCESS_KEY":      "secret-key",
		"STORAGE_FORCE_PATH_STYLE":       "true",
		"STORAGE_MANAGED_PREFIX":         "videos/",
		"STORAGE_UPLOAD_URL_TTL":         "15m",
		"STORAGE_READ_URL_TTL":           "10m",
		"VIDEO_IDEMPOTENCY_TTL":          "24h",
		"VIDEO_IDEMPOTENCY_HMAC_KEY":     "12345678901234567890123456789012",
		"VIDEO_IDEMPOTENCY_KEY_MAX_BYTES": "128",
	}

	for name, value := range values {
		t.Setenv(name, value)
	}
}
