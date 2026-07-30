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
	if err := db.AutoMigrate(&entity.RefreshToken{}); err != nil {
		return fmt.Errorf("auto migrate refresh tokens table: %w", err)
	}
	return nil
}
