package repository

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type IHealthRepository interface {
	Check(ctx context.Context) error
}

type healthRepository struct {
	db      *gorm.DB
	redis   *redis.Client
	storage IObjectStorageRepository
}

func NewHealthRepository(db *gorm.DB, redisClient *redis.Client, storage IObjectStorageRepository) IHealthRepository {
	return &healthRepository{
		db:      db,
		redis:   redisClient,
		storage: storage,
	}
}

func (r *healthRepository) Check(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("get postgres connection pool: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	if err := r.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}

	if err := r.storage.CheckHealth(ctx); err != nil {
		return fmt.Errorf("check object storage: %w", err)
	}

	return nil
}
