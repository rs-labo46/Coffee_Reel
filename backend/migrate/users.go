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
	if db.Migrator().HasTable(&entity.User{}) {
		return nil
	}
	if err := db.Migrator().CreateTable(&entity.User{}); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}
	return nil
}
