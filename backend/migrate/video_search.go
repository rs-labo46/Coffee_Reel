package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func MigrateVideoSearch(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm").Error; err != nil {
		return fmt.Errorf("enable pg_trgm extension: %w", err)
	}

	return createIndexIfMissing(
		db,
		"idx_videos_public_title_trgm",
		"CREATE INDEX idx_videos_public_title_trgm ON videos USING GIN (lower(title) gin_trgm_ops) WHERE processing_status = 'ready' AND publish_status = 'published' AND deleted_at IS NULL",
	)
}
