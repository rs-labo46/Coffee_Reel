//go:build integration

package repository

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/migrate"

	"gorm.io/gorm"
)

var searchLikeFixtureCounter atomic.Uint64

func openSearchLikeIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openPostgresIntegrationDB(t)

	migrations := []struct {
		name string
		run  func(*gorm.DB) error
	}{
		{name: "videos", run: migrate.MigrateVideos},
		{name: "video_output_metas", run: migrate.MigrateVideoOutputMetas},
		{name: "saved_videos", run: migrate.MigrateSavedVideos},
		{name: "video_likes", run: migrate.MigrateVideoLikes},
		{name: "video_search", run: migrate.MigrateVideoSearch},
	}
	for _, migration := range migrations {
		if err := migration.run(db); err != nil {
			t.Fatalf("migrate %s: %v", migration.name, err)
		}
	}
	return db
}

func createSearchLikeUser(t *testing.T, db *gorm.DB, status entity.UserStatus) entity.User {
	t.Helper()
	n := searchLikeFixtureCounter.Add(1)
	now := time.Now()
	user := entity.User{
		Name:         fmt.Sprintf("user-%d", n),
		Email:        fmt.Sprintf("search-like-%d@example.com", n),
		PasswordHash: "hash",
		Role:         entity.RoleUser,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createSearchLikeVideo(
	t *testing.T,
	db *gorm.DB,
	userID uint64,
	title string,
	category entity.CategoryCode,
	processing entity.VideoProcessingStatus,
	publish entity.VideoPublishStatus,
	createdAt time.Time,
	deleted bool,
) entity.Video {
	t.Helper()
	n := searchLikeFixtureCounter.Add(1)
	video := entity.Video{
		UserID:            userID,
		Category:          category,
		Title:             title,
		Description:       "description",
		OriginalObjectKey: fmt.Sprintf("videos/source/search-like-%d.mp4", n),
		UploadExpiresAt:   createdAt.Add(15 * time.Minute),
		ProcessingStatus:  processing,
		PublishStatus:     publish,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
	if deleted {
		deletedAt := createdAt.Add(time.Second)
		video.DeletedAt = &deletedAt
		video.UpdatedAt = deletedAt
	}
	if err := db.Create(&video).Error; err != nil {
		t.Fatalf("create video %q: %v", title, err)
	}
	return video
}

func createSearchLikeOutput(t *testing.T, db *gorm.DB, videoID uint64, createdAt time.Time) entity.OutputVideoMeta {
	t.Helper()
	n := searchLikeFixtureCounter.Add(1)
	meta := entity.OutputVideoMeta{
		VideoID:            videoID,
		VideoObjectKey:     fmt.Sprintf("videos/%d/output/search-like-%d.mp4", videoID, n),
		ThumbnailObjectKey: fmt.Sprintf("videos/%d/thumbnail/search-like-%d.jpg", videoID, n),
		Container:          "mp4",
		Width:              720,
		Height:             1280,
		FrameRate:          30,
		VideoCodec:         "h264",
		HasAudio:           true,
		AudioCodec:         "aac",
		CreatedAt:          createdAt,
	}
	if err := db.Create(&meta).Error; err != nil {
		t.Fatalf("create output meta: %v", err)
	}
	return meta
}

func createPublicSearchLikeVideo(t *testing.T, db *gorm.DB, userID uint64, title string, category entity.CategoryCode, createdAt time.Time) entity.Video {
	t.Helper()
	video := createSearchLikeVideo(
		t,
		db,
		userID,
		title,
		category,
		entity.VideoProcessingReady,
		entity.VideoPublishPublished,
		createdAt,
		false,
	)
	createSearchLikeOutput(t, db, video.ID, createdAt)
	return video
}
