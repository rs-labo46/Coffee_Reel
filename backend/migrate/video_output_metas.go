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

	if err := addForeignKey(
		db,
		"video_output_metas",
		"fk_video_output_metas_video",
		"video_id",
		"videos",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	checks := []migrationCheck{
		{
			name:       "chk_output_keys",
			expression: "char_length(btrim(video_object_key)) > 0 AND char_length(btrim(thumbnail_object_key)) > 0 AND btrim(video_object_key) <> btrim(thumbnail_object_key)",
		},
		{
			name:       "chk_output_container",
			expression: "container = 'mp4'",
		},
		{
			name:       "chk_output_resolution",
			expression: "width = 720 AND height = 1280",
		},
		{
			name:       "chk_output_frame_rate",
			expression: "frame_rate > 0 AND frame_rate <= 30",
		},
		{
			name:       "chk_output_video_codec",
			expression: "video_codec = 'h264'",
		},
		{
			name:       "chk_output_audio",
			expression: "(has_audio AND audio_codec = 'aac') OR (NOT has_audio AND audio_codec = '')",
		},
	}
	if err := addChecks(db, "video_output_metas", checks); err != nil {
		return err
	}

	indexes := []migrationIndex{
		{
			name:      "uq_video_output_metas_video_id",
			statement: "CREATE UNIQUE INDEX uq_video_output_metas_video_id ON video_output_metas (video_id)",
		},
		{
			name:      "uq_video_output_metas_video_object_key",
			statement: "CREATE UNIQUE INDEX uq_video_output_metas_video_object_key ON video_output_metas (video_object_key)",
		},
		{
			name:      "uq_video_output_metas_thumbnail_object_key",
			statement: "CREATE UNIQUE INDEX uq_video_output_metas_thumbnail_object_key ON video_output_metas (thumbnail_object_key)",
		},
	}
	return createIndexesIfMissing(db, indexes)
}
