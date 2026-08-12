package migrate

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const sourceVideoSizeConstraint = "chk_source_size"

func MigrateVideoSourceSizeLimit(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if !db.Migrator().HasTable("video_source_metas") {
		return fmt.Errorf("video source metas table is required")
	}

	current, err := sourceVideoSizeConstraintDefinition(db)
	if err != nil {
		return err
	}
	if strings.Contains(current, "50000000") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
ALTER TABLE video_source_metas
DROP CONSTRAINT IF EXISTS chk_source_size
`).Error; err != nil {
			return fmt.Errorf("drop %s: %w", sourceVideoSizeConstraint, err)
		}

		if err := tx.Exec(`
ALTER TABLE video_source_metas
ADD CONSTRAINT chk_source_size CHECK (size_bytes BETWEEN 1 AND 50000000)
`).Error; err != nil {
			return fmt.Errorf("add %s: %w", sourceVideoSizeConstraint, err)
		}

		return nil
	})
}

func sourceVideoSizeConstraintDefinition(db *gorm.DB) (string, error) {
	var definition string

	if err := db.Raw(`
SELECT COALESCE(pg_get_constraintdef(c.oid), '')
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = current_schema()
  AND t.relname = 'video_source_metas'
  AND c.conname = ?
  AND c.contype = 'c'
`, sourceVideoSizeConstraint).Scan(&definition).Error; err != nil {
		return "", fmt.Errorf("check %s: %w", sourceVideoSizeConstraint, err)
	}

	return definition, nil
}
