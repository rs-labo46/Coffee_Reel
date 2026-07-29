package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateSavedVideos(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if !db.Migrator().HasTable(&entity.SavedVideo{}) {
		if err := db.Migrator().CreateTable(&entity.SavedVideo{}); err != nil {
			return fmt.Errorf("create saved videos table: %w", err)
		}
	}

	if err := addForeignKey(
		db,
		"saved_videos",
		"fk_saved_videos_user",
		"user_id",
		"users",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}
	if err := addForeignKey(
		db,
		"saved_videos",
		"fk_saved_videos_video",
		"video_id",
		"videos",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	if err := dropIndexIfExists(db, "idx_saved_videos_list"); err != nil {
		return err
	}

	indexes := []migrationIndex{
		{
			name:      "uq_saved_videos_user_video",
			statement: "CREATE UNIQUE INDEX uq_saved_videos_user_video ON saved_videos (user_id, video_id)",
		},
		{
			name:      "idx_saved_videos_user_list",
			statement: "CREATE INDEX idx_saved_videos_user_list ON saved_videos (user_id, created_at DESC, id DESC)",
		},
		{
			name:      "idx_saved_videos_video",
			statement: "CREATE INDEX idx_saved_videos_video ON saved_videos (video_id)",
		},
	}
	return createIndexesIfMissing(db, indexes)
}
