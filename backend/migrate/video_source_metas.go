package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateVideoSourceMetas(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if !db.Migrator().HasTable(&entity.SourceVideoMeta{}) {
		if err := db.Migrator().CreateTable(&entity.SourceVideoMeta{}); err != nil {
			return fmt.Errorf("create video source metas table: %w", err)
		}
	}
	if err := addForeignKey(db, "video_source_metas", "fk_video_source_metas_video", "video_id", "videos", "id", "CASCADE"); err != nil {
		return err
	}
	checks := []struct{ name, expression string }{
		{"chk_video_source_metas_size", "size_bytes BETWEEN 1 AND 30000000"},
		{"chk_video_source_metas_duration", "duration_millis BETWEEN 1 AND 10000"},
		{"chk_video_source_metas_resolution", "width BETWEEN 1 AND 1080 AND height BETWEEN 1 AND 1920"},
		{"chk_video_source_metas_aspect", "width * 16 = height * 9"},
		{"chk_video_source_metas_frame_rate", "frame_rate > 0 AND frame_rate <= 60"},
		{"chk_video_source_metas_mime", "mime_type IN ('video/mp4','video/quicktime')"},
		{"chk_video_source_metas_container", "container IN ('mp4','mov')"},
		{"chk_video_source_metas_audio", "(has_audio AND char_length(audio_codec) > 0) OR (NOT has_audio AND audio_codec = '')"},
	}
	for _, check := range checks {
		if err := addCheck(db, "video_source_metas", check.name, check.expression); err != nil {
			return err
		}
	}
	return nil
}
