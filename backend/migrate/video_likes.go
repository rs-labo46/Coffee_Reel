package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateVideoLikes(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if err := db.AutoMigrate(&entity.VideoLike{}); err != nil {
		return fmt.Errorf("auto migrate video likes table: %w", err)
	}

	if err := addForeignKey(
		db,
		"video_likes",
		"fk_video_likes_user",
		"user_id",
		"users",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	if err := addForeignKey(
		db,
		"video_likes",
		"fk_video_likes_video",
		"video_id",
		"videos",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	return nil
}
