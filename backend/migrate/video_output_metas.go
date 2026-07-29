package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateVideoOutputMetas(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if !db.Migrator().HasTable(&entity.OutputVideoMeta{}) {
		if err := db.Migrator().CreateTable(&entity.OutputVideoMeta{}); err != nil {
			return fmt.Errorf("create video output metas table: %w", err)
		}
	}
	if err := addForeignKey(db, "video_output_metas", "fk_video_output_metas_video", "video_id", "videos", "id", "CASCADE"); err != nil {
		return err
	}
	checks := []struct{ name, expression string }{
		{"chk_video_output_metas_keys", "char_length(video_object_key) > 0 AND char_length(thumbnail_object_key) > 0 AND video_object_key <> thumbnail_object_key"},
		{"chk_video_output_metas_format", "container = 'mp4' AND width = 720 AND height = 1280 AND frame_rate > 0 AND frame_rate <= 30 AND video_codec = 'h264'"},
		{"chk_video_output_metas_audio", "(has_audio AND audio_codec = 'aac') OR (NOT has_audio AND audio_codec = '')"},
	}
	for _, check := range checks {
		if err := addCheck(db, "video_output_metas", check.name, check.expression); err != nil {
			return err
		}
	}
	return nil
}
