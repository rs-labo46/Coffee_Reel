package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateRefreshTokens(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if db.Migrator().HasTable(&entity.RefreshToken{}) {
		return nil
	}
	if err := db.Migrator().CreateTable(&entity.RefreshToken{}); err != nil {
		return fmt.Errorf("create refresh tokens table: %w", err)
	}
	return nil
}
