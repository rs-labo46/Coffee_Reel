package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"coffee-reel/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IVideoLikeRepository interface {
	Like(ctx context.Context, userID, videoID uint64, now time.Time) (VideoLikeState, error)
	Unlike(ctx context.Context, userID, videoID uint64) (VideoLikeState, error)
}

type VideoLikeState struct {
	VideoID   uint64
	LikeCount int64
	IsLiked   bool
}

type videoLikeRepository struct {
	db *gorm.DB
}

func NewVideoLikeRepository(db *gorm.DB) IVideoLikeRepository {
	return &videoLikeRepository{db: db}
}

func (r *videoLikeRepository) Like(ctx context.Context, userID, videoID uint64, now time.Time) (VideoLikeState, error) {
	if userID == 0 || videoID == 0 || now.IsZero() {
		return VideoLikeState{}, entity.ErrInvalidInput
	}

	var state VideoLikeState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id = ?", userID).
			Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrUnauthorized
			}
			return fmt.Errorf("lock user for video like: %w", err)
		}
		if !user.IsActive() {
			return entity.ErrUserSuspended
		}

		if err := lockPublicVideoForLike(tx, videoID); err != nil {
			return err
		}

		like, err := entity.NewVideoLike(userID, videoID, now)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "video_id"},
			},
			DoNothing: true,
		}).Create(like).Error; err != nil {
			return fmt.Errorf("create video like: %w", err)
		}

		count, err := countVideoLikes(tx, videoID)
		if err != nil {
			return err
		}
		state = VideoLikeState{
			VideoID:   videoID,
			LikeCount: count,
			IsLiked:   true,
		}
		return nil
	})
	if err != nil {
		return VideoLikeState{}, err
	}

	return state, nil
}

func (r *videoLikeRepository) Unlike(ctx context.Context, userID, videoID uint64) (VideoLikeState, error) {
	if userID == 0 || videoID == 0 {
		return VideoLikeState{}, entity.ErrInvalidInput
	}

	var state VideoLikeState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPublicVideoForLike(tx, videoID); err != nil {
			return err
		}

		if err := tx.
			Where("user_id = ? AND video_id = ?", userID, videoID).
			Delete(&entity.VideoLike{}).Error; err != nil {
			return fmt.Errorf("delete video like: %w", err)
		}

		count, err := countVideoLikes(tx, videoID)
		if err != nil {
			return err
		}
		state = VideoLikeState{
			VideoID:   videoID,
			LikeCount: count,
			IsLiked:   false,
		}
		return nil
	})
	if err != nil {
		return VideoLikeState{}, err
	}

	return state, nil
}

func lockPublicVideoForLike(tx *gorm.DB, videoID uint64) error {
	var video entity.Video
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
		Where("id = ?", videoID).
		Take(&video).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return entity.ErrVideoNotFound
		}
		return fmt.Errorf("lock public video for like: %w", err)
	}
	if !video.CanBeViewedPublicly() {
		return entity.ErrVideoNotFound
	}
	return nil
}

func countVideoLikes(tx *gorm.DB, videoID uint64) (int64, error) {
	var count int64
	if err := tx.Model(&entity.VideoLike{}).
		Where("video_id = ?", videoID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count video likes: %w", err)
	}
	return count, nil
}
