package entity

import "time"

type Environment string

const (
	EnvironmentDevelop    Environment = "develop"
	EnvironmentProduction Environment = "production"
)

func (e Environment) IsValid() bool {
	return e == EnvironmentDevelop || e == EnvironmentProduction
}

func (e Environment) IsProduction() bool {
	return e == EnvironmentProduction
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string `json:"-"`
	DB       int
}

type ObjectStorageConfig struct {
	Endpoint        string
	PresignEndpoint string
	Region          string
	Bucket          string
	AccessKeyID     string `json:"-"`
	SecretAccessKey string `json:"-"`
	ManagedPrefix   string
	ForcePathStyle  bool
	RequireHTTPS    bool
}

type Config struct {
	Environment                 Environment
	Port                        string
	DatabaseURL                 string `json:"-"`
	Redis                       RedisConfig
	FrontendURL                 string
	CookieSecure                bool
	CookieDomain                string
	JWTSecret                   string `json:"-"`
	RefreshTokenHMACKey         string `json:"-"`
	RateLimitHMACKey            string `json:"-"`
	Storage                     ObjectStorageConfig
	StorageUploadURLTTL         time.Duration
	StorageReadURLTTL           time.Duration
	VideoIdempotencyTTL         time.Duration
	VideoIdempotencyHMACKey     string `json:"-"`
	VideoIdempotencyKeyMaxBytes int
}
