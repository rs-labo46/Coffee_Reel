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
	if !db.Migrator().HasTable(&entity.User{}) {
		if err := db.Migrator().CreateTable(&entity.User{}); err != nil {
			return fmt.Errorf("create users table: %w", err)
		}
	}

	if !db.Migrator().HasIndex(&entity.User{}, "idx_users_admin_list") {
		if err := db.Migrator().CreateIndex(&entity.User{}, "idx_users_admin_list"); err != nil {
			return fmt.Errorf("create users admin list index: %w", err)
		}
	}

	return nil
}
