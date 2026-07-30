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

	if err := db.AutoMigrate(&entity.SavedVideo{}); err != nil {
		return fmt.Errorf("auto migrate saved videos table: %w", err)
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

	return nil
}
