package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

type migrationCheck struct {
	name       string
	expression string
}

type migrationIndex struct {
	name      string
	statement string
}

func MigrateVideos(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if err := db.AutoMigrate(&entity.Video{}); err != nil {
		return fmt.Errorf("auto migrate videos table: %w", err)
	}

	if err := addForeignKey(db, "videos", "fk_videos_user", "user_id", "users", "id", "RESTRICT"); err != nil {
		return err
	}

	checks := []migrationCheck{
		{name: "chk_videos_category", expression: "category IN ('brewing','roasting','latte_art','beans','equipment')"},
		{name: "chk_videos_title", expression: "title = btrim(title) AND char_length(title) BETWEEN 1 AND 100"},
		{name: "chk_videos_description", expression: "char_length(description) <= 1000"},
		{name: "chk_videos_original_key", expression: "char_length(btrim(original_object_key)) > 0"},
		{name: "chk_videos_processing_status", expression: "processing_status IN ('uploading','expired','uploaded','processing','ready','failed')"},
		{name: "chk_videos_publish_status", expression: "publish_status IN ('private','published','hidden')"},
		{name: "chk_videos_upload_expiry", expression: "upload_expires_at > created_at"},
		{name: "chk_videos_timestamps", expression: "updated_at >= created_at AND (deleted_at IS NULL OR deleted_at >= created_at)"},
		{name: "chk_videos_non_ready_private", expression: "processing_status = 'ready' OR publish_status = 'private'"},
		{name: "chk_videos_public_ready", expression: "publish_status NOT IN ('published','hidden') OR (processing_status = 'ready' AND deleted_at IS NULL)"},
		{name: "chk_videos_deleted_private", expression: "deleted_at IS NULL OR publish_status = 'private'"},
	}
	if err := addChecks(db, "videos", checks); err != nil {
		return err
	}

	if err := dropIndexIfExists(db, "idx_videos_upload_expiration"); err != nil {
		return err
	}

	indexes := []migrationIndex{
		{
			name:      "idx_videos_public_feed",
			statement: "CREATE INDEX idx_videos_public_feed ON videos (created_at DESC, id DESC) WHERE processing_status = 'ready' AND publish_status = 'published' AND deleted_at IS NULL",
		},
		{
			name:      "idx_videos_public_category",
			statement: "CREATE INDEX idx_videos_public_category ON videos (category, created_at DESC, id DESC) WHERE processing_status = 'ready' AND publish_status = 'published' AND deleted_at IS NULL",
		},
		{
			name:      "idx_videos_admin_list",
			statement: "CREATE INDEX idx_videos_admin_list ON videos (processing_status, publish_status, created_at DESC, id DESC)",
		},
		{
			name:      "idx_videos_upload_expiry",
			statement: "CREATE INDEX idx_videos_upload_expiry ON videos (upload_expires_at, id) WHERE processing_status = 'uploading' AND deleted_at IS NULL",
		},
	}
	return createIndexesIfMissing(db, indexes)
}

func addChecks(db *gorm.DB, table string, checks []migrationCheck) error {
	for _, check := range checks {
		if err := addCheck(db, table, check.name, check.expression); err != nil {
			return err
		}
	}
	return nil
}

func addCheck(db *gorm.DB, table, name, expression string) error {
	exists, err := constraintExists(db, table, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	statement := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)", table, name, expression)
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	return nil
}

func addForeignKey(db *gorm.DB, table, name, column, referenceTable, referenceColumn, onDelete string) error {
	exists, err := constraintExists(db, table, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	statement := fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s",
		table,
		name,
		column,
		referenceTable,
		referenceColumn,
		onDelete,
	)
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	return nil
}

func constraintExists(db *gorm.DB, table, name string) (bool, error) {
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = ? AND constraint_name = ?`,
		table,
		name,
	).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("check constraint %s: %w", name, err)
	}
	return count > 0, nil
}

func createIndexesIfMissing(db *gorm.DB, indexes []migrationIndex) error {
	for _, index := range indexes {
		if err := createIndexIfMissing(db, index.name, index.statement); err != nil {
			return err
		}
	}
	return nil
}

func createIndexIfMissing(db *gorm.DB, name, statement string) error {
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`,
		name).Scan(&count).Error; err != nil {
		return fmt.Errorf("check index %s: %w", name, err)
	}
	if count > 0 {
		return nil
	}
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("create index %s: %w", name, err)
	}
	return nil
}

func dropIndexIfExists(db *gorm.DB, name string) error {
	if err := db.Exec("DROP INDEX IF EXISTS " + name).Error; err != nil {
		return fmt.Errorf("drop index %s: %w", name, err)
	}
	return nil
}
