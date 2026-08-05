package migrate

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const adminAuditActionStateConstraint = "chk_audit_action_state"

func MigrateAdminAuditLogVideoActions(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if !db.Migrator().HasTable("admin_audit_logs") {
		return fmt.Errorf("admin audit logs table is required")
	}

	current, err := hasAdminAuditVideoActions(db)
	if err != nil {
		return err
	}
	if current {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
ALTER TABLE admin_audit_logs
DROP CONSTRAINT IF EXISTS chk_audit_action_state
`).Error; err != nil {
			return fmt.Errorf("drop %s: %w", adminAuditActionStateConstraint, err)
		}

		statement := `
ALTER TABLE admin_audit_logs
ADD CONSTRAINT chk_audit_action_state CHECK (
	(action = 'user_suspend'
		AND target_type = 'user'
		AND before_status = 'active'
		AND after_status = 'suspended')
	OR
	(action = 'user_resume'
		AND target_type = 'user'
		AND before_status = 'suspended'
		AND after_status = 'active')
	OR
	(action = 'video_hide_by_user_suspension'
		AND target_type = 'video'
		AND before_status = 'published'
		AND after_status = 'hidden')
	OR
	(action = 'video_hide'
		AND target_type = 'video'
		AND before_status = 'published'
		AND after_status = 'hidden')
	OR
	(action = 'video_restore'
		AND target_type = 'video'
		AND before_status = 'hidden'
		AND after_status = 'published')
)`

		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("add %s: %w", adminAuditActionStateConstraint, err)
		}
		return nil
	})
}

func hasAdminAuditVideoActions(db *gorm.DB) (bool, error) {
	var definition string

	if err := db.Raw(`
SELECT pg_get_constraintdef(c.oid)
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = current_schema()
  AND t.relname = 'admin_audit_logs'
  AND c.conname = ?
  AND c.contype = 'c'
`, adminAuditActionStateConstraint).Scan(&definition).Error; err != nil {
		return false, fmt.Errorf(
			"check %s: %w",
			adminAuditActionStateConstraint,
			err,
		)
	}

	return strings.Contains(definition, "'video_hide'") &&
		strings.Contains(definition, "'video_restore'"), nil
}
