package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateStorageCleanupJobs(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if !db.Migrator().HasTable(&entity.StorageCleanupJob{}) {
		if err := db.Migrator().CreateTable(&entity.StorageCleanupJob{}); err != nil {
			return fmt.Errorf("create storage cleanup jobs table: %w", err)
		}
	}

	if err := addForeignKey(
		db,
		"storage_cleanup_jobs",
		"fk_storage_cleanup_jobs_video",
		"video_id",
		"videos",
		"id",
		"SET NULL",
	); err != nil {
		return err
	}

	checks := []migrationCheck{
		{
			name:       "chk_cleanup_object_key",
			expression: "char_length(btrim(object_key)) > 0",
		},
		{
			name:       "chk_cleanup_asset_type",
			expression: "asset_type IN ('original','processed','thumbnail','unknown')",
		},
		{
			name:       "chk_cleanup_cause",
			expression: "cause IN ('video_delete','upload_expired','process_failed','rollback_cleanup','orphan_detected')",
		},
		{
			name:       "chk_cleanup_status",
			expression: "status IN ('queued','running','retry_wait','succeeded','failed')",
		},
		{
			name:       "chk_cleanup_attempts",
			expression: "max_attempts = 4 AND attempt_count BETWEEN 0 AND max_attempts",
		},
		{
			name:       "chk_cleanup_queued",
			expression: "status <> 'queued' OR (attempt_count = 0 AND started_at IS NULL AND finished_at IS NULL)",
		},
		{
			name:       "chk_cleanup_running",
			expression: "status <> 'running' OR (attempt_count >= 1 AND started_at IS NOT NULL AND finished_at IS NULL)",
		},
		{
			name:       "chk_cleanup_retry",
			expression: "status <> 'retry_wait' OR (attempt_count >= 1 AND started_at IS NOT NULL AND finished_at IS NULL AND available_at > updated_at)",
		},
		{
			name:       "chk_cleanup_terminal",
			expression: "status NOT IN ('succeeded','failed') OR finished_at IS NOT NULL",
		},
		{
			name:       "chk_cleanup_non_terminal",
			expression: "status IN ('succeeded','failed') OR finished_at IS NULL",
		},
		{
			name: "chk_cleanup_error",
			expression: "(last_error_code = '' AND last_error_message = '') OR " +
				"(last_error_code ~ '^[a-z0-9_]{1,64}$' AND char_length(btrim(last_error_message)) BETWEEN 1 AND 500)",
		},
		{
			name:       "chk_cleanup_retry_error",
			expression: "status <> 'retry_wait' OR last_error_code <> ''",
		},
		{
			name:       "chk_cleanup_failed_error",
			expression: "status <> 'failed' OR last_error_code <> ''",
		},
		{
			name:       "chk_cleanup_succeeded_error",
			expression: "status <> 'succeeded' OR (last_error_code = '' AND last_error_message = '')",
		},
		{
			name:       "chk_cleanup_timestamps",
			expression: "available_at >= created_at AND updated_at >= created_at AND (started_at IS NULL OR started_at >= created_at) AND (finished_at IS NULL OR finished_at >= created_at) AND (started_at IS NULL OR finished_at IS NULL OR finished_at >= started_at)",
		},
	}
	if err := addChecks(db, "storage_cleanup_jobs", checks); err != nil {
		return err
	}

	for _, name := range []string{
		"idx_storage_cleanup_jobs_claim",
		"idx_storage_cleanup_jobs_timeout",
		"idx_storage_cleanup_jobs_video_id",
		"uq_storage_cleanup_jobs_unfinished_object",
		"idx_storage_cleanup_jobs_timeout_running",
	} {
		if err := dropIndexIfExists(db, name); err != nil {
			return err
		}
	}

	indexes := []migrationIndex{
		{
			name:      "uq_cleanup_unfinished_object",
			statement: "CREATE UNIQUE INDEX uq_cleanup_unfinished_object ON storage_cleanup_jobs (object_key) WHERE status IN ('queued','running','retry_wait')",
		},
		{
			name:      "idx_cleanup_claim",
			statement: "CREATE INDEX idx_cleanup_claim ON storage_cleanup_jobs (available_at, id) WHERE status IN ('queued','retry_wait')",
		},
		{
			name:      "idx_cleanup_running_timeout",
			statement: "CREATE INDEX idx_cleanup_running_timeout ON storage_cleanup_jobs (started_at, id) WHERE status = 'running'",
		},
		{
			name:      "idx_cleanup_failed",
			statement: "CREATE INDEX idx_cleanup_failed ON storage_cleanup_jobs (updated_at DESC, id DESC) WHERE status = 'failed'",
		},
		{
			name:      "idx_cleanup_video",
			statement: "CREATE INDEX idx_cleanup_video ON storage_cleanup_jobs (video_id, created_at DESC) WHERE video_id IS NOT NULL",
		},
	}
	return createIndexesIfMissing(db, indexes)
}
