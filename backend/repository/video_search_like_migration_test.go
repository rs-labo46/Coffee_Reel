//go:build integration

package repository

import (
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestSearchLikeMigrationsCreateConstraintsAndSearchIndex(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)

	if !db.Migrator().HasTable(&entity.VideoLike{}) {
		t.Fatal("video_likes table was not created")
	}

	for _, constraint := range []string{
		"fk_video_likes_user",
		"fk_video_likes_video",
	} {
		var count int64
		if err := db.Raw(`
			SELECT COUNT(*)
			FROM information_schema.table_constraints
			WHERE table_schema = current_schema()
			  AND table_name = 'video_likes'
			  AND constraint_name = ?`, constraint).Scan(&count).Error; err != nil {
			t.Fatalf("check constraint %s: %v", constraint, err)
		}
		if count != 1 {
			t.Fatalf("constraint %s count = %d, want 1", constraint, count)
		}
	}

	for _, column := range []string{"user_id", "video_id", "created_at"} {
		var nullable string
		if err := db.Raw(`
			SELECT is_nullable
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'video_likes'
			  AND column_name = ?`, column).Scan(&nullable).Error; err != nil {
			t.Fatalf("check column %s: %v", column, err)
		}
		if nullable != "NO" {
			t.Fatalf("column %s is_nullable = %q, want NO", column, nullable)
		}
	}

	indexExpectations := map[string][]string{
		"uq_video_likes_user_video": {"create unique index", "(user_id, video_id)"},
		"idx_video_likes_video":     {"(video_id, created_at desc)"},
		"idx_video_likes_user":      {"(user_id, created_at desc, id desc)"},
	}
	for index, expectedParts := range indexExpectations {
		var indexDef string
		if err := db.Raw(`
			SELECT indexdef
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = 'video_likes'
			  AND indexname = ?`, index).Scan(&indexDef).Error; err != nil {
			t.Fatalf("read index %s: %v", index, err)
		}
		if indexDef == "" {
			t.Fatalf("index %s was not created", index)
		}
		normalized := strings.ToLower(strings.ReplaceAll(indexDef, `"`, ""))
		for _, part := range expectedParts {
			if !strings.Contains(normalized, part) {
				t.Fatalf("index %s definition missing %q: %s", index, part, indexDef)
			}
		}
	}

	var extensionCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_extension WHERE extname = 'pg_trgm'`).Scan(&extensionCount).Error; err != nil {
		t.Fatalf("check pg_trgm extension: %v", err)
	}
	if extensionCount != 1 {
		t.Fatalf("pg_trgm count = %d, want 1", extensionCount)
	}

	var indexDef string
	if err := db.Raw(`
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = 'idx_videos_public_title_trgm'`).Scan(&indexDef).Error; err != nil {
		t.Fatalf("read title trigram index: %v", err)
	}
	lower := strings.ToLower(indexDef)
	for _, part := range []string{
		"using gin",
		"lower(",
		"title",
		"gin_trgm_ops",
		"processing_status",
		"ready",
		"publish_status",
		"published",
		"deleted_at is null",
	} {
		if !strings.Contains(lower, part) {
			t.Fatalf("title index definition missing %q: %s", part, indexDef)
		}
	}
}

func TestVideoLikeUniqueAllowsDifferentUserOrVideo(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user1 := createSearchLikeUser(t, db, entity.StatusActive)
	user2 := createSearchLikeUser(t, db, entity.StatusActive)
	now := time.Now()
	video1 := createPublicSearchLikeVideo(t, db, user1.ID, "video one", entity.CategoryBrewing, now)
	video2 := createPublicSearchLikeVideo(t, db, user1.ID, "video two", entity.CategoryRoasting, now.Add(time.Second))

	first := entity.VideoLike{UserID: user1.ID, VideoID: video1.ID, CreatedAt: now}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first like: %v", err)
	}
	duplicate := entity.VideoLike{UserID: user1.ID, VideoID: video1.ID, CreatedAt: now.Add(time.Second)}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("duplicate user/video like succeeded")
	}
	if err := db.Create(&entity.VideoLike{UserID: user2.ID, VideoID: video1.ID, CreatedAt: now}).Error; err != nil {
		t.Fatalf("different user same video: %v", err)
	}
	if err := db.Create(&entity.VideoLike{UserID: user1.ID, VideoID: video2.ID, CreatedAt: now}).Error; err != nil {
		t.Fatalf("same user different video: %v", err)
	}

	var count int64
	if err := db.Model(&entity.VideoLike{}).Count(&count).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if count != 3 {
		t.Fatalf("like count = %d, want 3", count)
	}
}
