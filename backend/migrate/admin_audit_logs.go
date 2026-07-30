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
	if err := db.AutoMigrate(&entity.AdminAuditLog{}); err != nil {
		return fmt.Errorf("auto migrate admin audit logs table: %w", err)
	}
	return nil
}
