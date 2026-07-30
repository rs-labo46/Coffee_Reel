package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateVideoProcessingJobs(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if err := db.AutoMigrate(&entity.VideoProcessingJob{}); err != nil {
		return fmt.Errorf("auto migrate video processing jobs table: %w", err)
	}

	if err := addForeignKey(
		db,
		"video_processing_jobs",
		"fk_video_processing_jobs_video",
		"video_id",
		"videos",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	checks := []migrationCheck{
		{
			name:       "chk_video_job_status",
			expression: "status IN ('queued','running','retry_wait','succeeded','failed','cancelled')",
		},
		{
			name:       "chk_video_job_attempts",
			expression: "max_attempts = 4 AND attempt_count BETWEEN 0 AND max_attempts",
		},
		{
			name:       "chk_video_job_queued",
			expression: "status <> 'queued' OR (attempt_count = 0 AND started_at IS NULL AND finished_at IS NULL)",
		},
		{
			name:       "chk_video_job_running",
			expression: "status <> 'running' OR (attempt_count >= 1 AND started_at IS NOT NULL AND finished_at IS NULL)",
		},
		{
			name:       "chk_video_job_retry",
			expression: "status <> 'retry_wait' OR (attempt_count >= 1 AND started_at IS NOT NULL AND finished_at IS NULL AND available_at > updated_at)",
		},
		{
			name:       "chk_video_job_terminal",
			expression: "status NOT IN ('succeeded','failed','cancelled') OR finished_at IS NOT NULL",
		},
		{
			name:       "chk_video_job_non_terminal",
			expression: "status IN ('succeeded','failed','cancelled') OR finished_at IS NULL",
		},
		{
			name: "chk_video_job_error",
			expression: "(last_error_code = '' AND last_error_message = '') OR " +
				"(last_error_code IN ('invalid_format','video_corrupt','duration_exceeded','size_exceeded','resolution_exceeded','invalid_aspect_ratio','frame_rate_exceeded','video_track_missing','processing_failed','storage_unavailable','worker_timeout') " +
				"AND char_length(btrim(last_error_message)) BETWEEN 1 AND 500)",
		},
		{
			name:       "chk_video_job_retry_error",
			expression: "status <> 'retry_wait' OR last_error_code <> ''",
		},
		{
			name:       "chk_video_job_failed_error",
			expression: "status <> 'failed' OR last_error_code <> ''",
		},
		{
			name:       "chk_video_job_succeeded_error",
			expression: "status <> 'succeeded' OR (last_error_code = '' AND last_error_message = '')",
		},
		{
			name:       "chk_video_job_timestamps",
			expression: "available_at >= created_at AND updated_at >= created_at AND (started_at IS NULL OR started_at >= created_at) AND (finished_at IS NULL OR finished_at >= created_at) AND (started_at IS NULL OR finished_at IS NULL OR finished_at >= started_at)",
		},
	}
	if err := addChecks(db, "video_processing_jobs", checks); err != nil {
		return err
	}

	for _, name := range []string{
		"idx_video_processing_jobs_claim",
		"idx_video_processing_jobs_timeout",
		"idx_video_processing_jobs_timeout_running",
	} {
		if err := dropIndexIfExists(db, name); err != nil {
			return err
		}
	}

	indexes := []migrationIndex{
		{
			name:      "idx_video_jobs_claim",
			statement: "CREATE INDEX idx_video_jobs_claim ON video_processing_jobs (available_at, id) WHERE status IN ('queued','retry_wait')",
		},
		{
			name:      "idx_video_jobs_running_timeout",
			statement: "CREATE INDEX idx_video_jobs_running_timeout ON video_processing_jobs (started_at, id) WHERE status = 'running'",
		},
		{
			name:      "idx_video_jobs_failed",
			statement: "CREATE INDEX idx_video_jobs_failed ON video_processing_jobs (updated_at DESC, id DESC) WHERE status = 'failed'",
		},
	}
	return createIndexesIfMissing(db, indexes)
}
