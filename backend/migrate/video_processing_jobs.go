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
	if !db.Migrator().HasTable(&entity.VideoProcessingJob{}) {
		if err := db.Migrator().CreateTable(&entity.VideoProcessingJob{}); err != nil {
			return fmt.Errorf("create video processing jobs table: %w", err)
		}
	}
	if err := addForeignKey(db, "video_processing_jobs", "fk_video_processing_jobs_video", "video_id", "videos", "id", "CASCADE"); err != nil {
		return err
	}
	checks := []struct{ name, expression string }{
		{"chk_video_processing_jobs_status", "status IN ('queued','running','retry_wait','succeeded','failed','cancelled')"},
		{"chk_video_processing_jobs_attempts", "attempt_count >= 0 AND max_attempts = 4 AND attempt_count <= max_attempts"},
		{"chk_video_processing_jobs_error_lengths", "char_length(last_error_code) <= 64 AND char_length(last_error_message) <= 500"},
	}
	for _, check := range checks {
		if err := addCheck(db, "video_processing_jobs", check.name, check.expression); err != nil {
			return err
		}
	}
	if err := createIndexIfMissing(db, "idx_video_processing_jobs_timeout_running", "CREATE INDEX idx_video_processing_jobs_timeout_running ON video_processing_jobs (started_at ASC, id ASC) WHERE status = 'running'"); err != nil {
		return err
	}
	return nil
}
