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

type ISavedVideoRepository interface {
	Save(ctx context.Context, userID, videoID uint64, now time.Time) error
	Remove(ctx context.Context, userID, videoID uint64) error
	ListByUser(ctx context.Context, userID uint64, limit int, cursor *SavedVideoCursor) (SavedVideoPage, error)
	Exists(ctx context.Context, userID, videoID uint64) (bool, error)
}
type SavedVideoCursor struct {
	CreatedAt time.Time
	ID        uint64
}

type SavedVideoPage struct {
	Items         []PublicVideoItem
	HasMore       bool
	LastCreatedAt time.Time
	LastID        uint64
}

type savedVideoRepository struct {
	db *gorm.DB
}

func NewSavedVideoRepository(db *gorm.DB) ISavedVideoRepository {
	return &savedVideoRepository{db: db}
}

func (r *savedVideoRepository) Save(ctx context.Context, userID, videoID uint64, now time.Time) error {
	if userID == 0 || videoID == 0 || now.IsZero() {
		return entity.ErrInvalidInput
	}
	now = now.UTC()

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ?", videoID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock video for save: %w", err)
		}
		if !video.CanBeViewedPublicly() {
			return entity.ErrVideoNotFound
		}

		saved, err := entity.NewSavedVideo(userID, videoID, now)
		if err != nil {
			return err
		}

		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "video_id"}}, DoNothing: true}).Create(saved).Error; err != nil {
			return fmt.Errorf("save video: %w", err)
		}

		return nil
	})
}

func (r *savedVideoRepository) Remove(ctx context.Context, userID, videoID uint64) error {
	if userID == 0 || videoID == 0 {
		return entity.ErrInvalidInput
	}

	if err := r.db.WithContext(ctx).Where("user_id = ? AND video_id = ?", userID, videoID).Delete(&entity.SavedVideo{}).Error; err != nil {
		return fmt.Errorf("remove saved video: %w", err)
	}

	return nil
}

func (r *savedVideoRepository) ListByUser(ctx context.Context, userID uint64, limit int, cursor *SavedVideoCursor) (SavedVideoPage, error) {
	if userID == 0 || limit < 1 || limit > 100 || (cursor != nil && (cursor.CreatedAt.IsZero() || cursor.ID == 0)) {
		return SavedVideoPage{}, entity.ErrInvalidInput
	}

	type savedRow struct {
		SavedID            uint64
		SavedCreatedAt     time.Time
		ID                 uint64
		UserID             uint64
		AuthorName         string
		Category           entity.CategoryCode
		Title              string
		Description        string
		VideoObjectKey     string
		ThumbnailObjectKey string
		CreatedAt          time.Time
	}

	rows := make([]savedRow, 0, limit+1)

	query := r.db.WithContext(ctx).Table("saved_videos").Select(`saved_videos.id AS saved_id, saved_videos.created_at AS saved_created_at,
		videos.id, videos.user_id, users.name AS author_name, videos.category, videos.title, videos.description,
		video_output_metas.video_object_key, video_output_metas.thumbnail_object_key, videos.created_at`).Joins("JOIN videos ON videos.id = saved_videos.video_id").Joins("JOIN users ON users.id = videos.user_id").Joins("JOIN video_output_metas ON video_output_metas.video_id = videos.id").Where("saved_videos.user_id = ?", userID).Where("videos.processing_status = ? AND videos.publish_status = ? AND videos.deleted_at IS NULL", entity.VideoProcessingReady, entity.VideoPublishPublished)

	if cursor != nil {
		query = query.Where("(saved_videos.created_at < ? OR (saved_videos.created_at = ? AND saved_videos.id < ?))", cursor.CreatedAt.UTC(), cursor.CreatedAt.UTC(), cursor.ID)
	}

	if err := query.Order("saved_videos.created_at DESC").Order("saved_videos.id DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return SavedVideoPage{}, fmt.Errorf("list saved videos: %w", err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items := make([]PublicVideoItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PublicVideoItem{
			ID:                 row.ID,
			UserID:             row.UserID,
			AuthorName:         row.AuthorName,
			Category:           row.Category,
			Title:              row.Title,
			Description:        row.Description,
			VideoObjectKey:     row.VideoObjectKey,
			ThumbnailObjectKey: row.ThumbnailObjectKey,
			CreatedAt:          row.CreatedAt.UTC(),
			IsSaved:            true,
		})
	}

	page := SavedVideoPage{
		Items:   items,
		HasMore: hasMore,
	}

	if len(rows) > 0 {
		last := rows[len(rows)-1]
		page.LastCreatedAt = last.SavedCreatedAt.UTC()
		page.LastID = last.SavedID
	}

	return page, nil
}

func (r *savedVideoRepository) Exists(ctx context.Context, userID, videoID uint64) (bool, error) {
	if userID == 0 || videoID == 0 {
		return false, entity.ErrInvalidInput
	}

	var count int64
	if err := r.db.WithContext(ctx).Model(&entity.SavedVideo{}).Where("user_id = ? AND video_id = ?", userID, videoID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check saved video: %w", err)
	}

	return count > 0, nil
}
