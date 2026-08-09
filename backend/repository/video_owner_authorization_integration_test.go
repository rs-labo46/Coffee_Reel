//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestVideoRepositorySetPrivateByOwnerRejectsOtherUserAsForbidden(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	owner := createSearchLikeUser(t, db, entity.StatusActive)
	other := createSearchLikeUser(t, db, entity.StatusActive)
	now := time.Now()
	video := createSearchLikeVideo(
		t,
		db,
		owner.ID,
		"owner authorization private test",
		entity.CategoryBrewing,
		entity.VideoProcessingReady,
		entity.VideoPublishPublished,
		now,
		false,
	)

	repository := NewVideoRepository(db)
	_, err := repository.SetPrivateByOwner(
		context.Background(),
		other.ID,
		video.ID,
		now.Add(time.Minute),
	)
	if !errors.Is(err, entity.ErrVideoForbidden) {
		t.Fatalf("SetPrivateByOwner() error = %v, want ErrVideoForbidden", err)
	}

	var reloaded entity.Video
	if err := db.First(&reloaded, video.ID).Error; err != nil {
		t.Fatalf("reload video: %v", err)
	}
	if reloaded.PublishStatus != entity.VideoPublishPublished {
		t.Fatalf("publish status = %q, want %q", reloaded.PublishStatus, entity.VideoPublishPublished)
	}
}

func TestVideoRepositoryRepublishByOwnerRejectsOtherUserAsForbidden(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	owner := createSearchLikeUser(t, db, entity.StatusActive)
	other := createSearchLikeUser(t, db, entity.StatusActive)
	now := time.Now()
	video := createSearchLikeVideo(
		t,
		db,
		owner.ID,
		"owner authorization republish test",
		entity.CategoryBrewing,
		entity.VideoProcessingReady,
		entity.VideoPublishPrivate,
		now,
		false,
	)

	repository := NewVideoRepository(db)
	_, err := repository.RepublishByOwner(
		context.Background(),
		other.ID,
		video.ID,
		now.Add(time.Minute),
	)
	if !errors.Is(err, entity.ErrVideoForbidden) {
		t.Fatalf("RepublishByOwner() error = %v, want ErrVideoForbidden", err)
	}

	var reloaded entity.Video
	if err := db.First(&reloaded, video.ID).Error; err != nil {
		t.Fatalf("reload video: %v", err)
	}
	if reloaded.PublishStatus != entity.VideoPublishPrivate {
		t.Fatalf("publish status = %q, want %q", reloaded.PublishStatus, entity.VideoPublishPrivate)
	}
}

func TestVideoRepositoryDeleteByOwnerRejectsOtherUserAsForbidden(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	owner := createSearchLikeUser(t, db, entity.StatusActive)
	other := createSearchLikeUser(t, db, entity.StatusActive)
	now := time.Now()
	video := createSearchLikeVideo(
		t,
		db,
		owner.ID,
		"owner authorization delete test",
		entity.CategoryBrewing,
		entity.VideoProcessingReady,
		entity.VideoPublishPublished,
		now,
		false,
	)

	repository := NewVideoRepository(db)
	err := repository.DeleteByOwner(
		context.Background(),
		other.ID,
		video.ID,
		now.Add(time.Minute),
	)
	if !errors.Is(err, entity.ErrVideoForbidden) {
		t.Fatalf("DeleteByOwner() error = %v, want ErrVideoForbidden", err)
	}

	var reloaded entity.Video
	if err := db.First(&reloaded, video.ID).Error; err != nil {
		t.Fatalf("reload video: %v", err)
	}
	if reloaded.DeletedAt != nil {
		t.Fatalf("deleted_at = %v, want nil", reloaded.DeletedAt)
	}
	if reloaded.PublishStatus != entity.VideoPublishPublished {
		t.Fatalf("publish status = %q, want %q", reloaded.PublishStatus, entity.VideoPublishPublished)
	}
}
