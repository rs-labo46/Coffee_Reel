package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateUsers(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		return fmt.Errorf("auto migrate users table: %w", err)
	}
	return nil
}
