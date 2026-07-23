package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateAdminAuditLogs(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if db.Migrator().HasTable(&entity.AdminAuditLog{}) {
		return nil
	}
	if err := db.Migrator().CreateTable(&entity.AdminAuditLog{}); err != nil {
		return fmt.Errorf("create admin audit logs table: %w", err)
	}
	return nil
}
