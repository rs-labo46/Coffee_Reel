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

	if err := addForeignKey(
		db,
		"video_source_metas",
		"fk_video_source_metas_video",
		"video_id",
		"videos",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	checks := []migrationCheck{
		{name: "chk_source_mime_container",
			expression: "(container = 'mp4' AND mime_type = 'video/mp4') OR (container = 'mov' AND mime_type = 'video/quicktime')",
		},
		{
			name:       "chk_source_size",
			expression: "size_bytes BETWEEN 1 AND 30000000",
		},
		{
			name:       "chk_source_duration",
			expression: "duration_millis BETWEEN 1 AND 10000",
		},
		{
			name:       "chk_source_resolution",
			expression: "width BETWEEN 1 AND 1080 AND height BETWEEN 1 AND 1920",
		},
		{
			name:       "chk_source_aspect_ratio",
			expression: "width * 16 = height * 9",
		},
		{
			name:       "chk_source_frame_rate",
			expression: "frame_rate > 0 AND frame_rate <= 60",
		},
		{
			name:       "chk_source_video_codec",
			expression: "char_length(btrim(video_codec)) > 0",
		},
		{
			name:       "chk_source_audio",
			expression: "(has_audio AND char_length(btrim(audio_codec)) > 0) OR (NOT has_audio AND audio_codec = '')",
		},
	}
	if err := addChecks(db, "video_source_metas", checks); err != nil {
		return err
	}

	return createIndexIfMissing(
		db,
		"uq_video_source_metas_video_id",
		"CREATE UNIQUE INDEX uq_video_source_metas_video_id ON video_source_metas (video_id)",
	)
}
