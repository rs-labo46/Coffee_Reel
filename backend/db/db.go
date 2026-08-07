package db

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewDB(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		NowFunc: func() time.Time {
			return time.Now()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return db, nil
}

func CloseDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get postgres connection pool: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close postgres connection pool: %w", err)
	}
	return nil
}
