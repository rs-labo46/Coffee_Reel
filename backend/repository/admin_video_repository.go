package repository

import (
	"coffee-reel/entity"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IAdminVideoRepository interface {
	List(ctx context.Context, limit int, cursor *AdminVideoCursor) (AdminVideoListResult, error)
	FindByID(ctx context.Context, videoID uint64) (*AdminVideoDetail, error)
	Hide(ctx context.Context, adminUserID, videoID uint64, reason, requestID string, now time.Time) (*AdminVideoState, error)
	Restore(ctx context.Context, adminUserID, videoID uint64, reason, requestID string, now time.Time) (*AdminVideoState, error)
}

type AdminVideoCursor struct {
	CreatedAt time.Time
	ID        uint64
}

type AdminVideoListItem struct {
	VideoID          uint64
	AuthorID         uint64
	AuthorName       string
	AuthorStatus     entity.UserStatus
	Title            string
	Description      string
	Category         entity.CategoryCode
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AdminVideoListResult struct {
	Items         []AdminVideoListItem
	HasMore       bool
	LastCreatedAt time.Time
	LastID        uint64
}

type AdminVideoOutputMeta struct {
	VideoObjectKey     string
	ThumbnailObjectKey string
}

type AdminVideoDetail struct {
	VideoID          uint64
	AuthorID         uint64
	AuthorName       string
	AuthorStatus     entity.UserStatus
	Title            string
	Description      string
	Category         entity.CategoryCode
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	OutputMeta       *AdminVideoOutputMeta `gorm:"-"`
}

type AdminVideoState struct {
	VideoID          uint64
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	UpdatedAt        time.Time
}

type adminVideoRepository struct {
	db *gorm.DB
}

func NewAdminVideoRepository(db *gorm.DB) IAdminVideoRepository {
	return &adminVideoRepository{db: db}
}

func (r *adminVideoRepository) List(ctx context.Context, limit int, cursor *AdminVideoCursor) (AdminVideoListResult, error) {
	if limit < 1 || limit > 100 ||
		(cursor != nil && (cursor.CreatedAt.IsZero() || cursor.ID == 0)) {
		return AdminVideoListResult{}, entity.ErrInvalidInput
	}

	items := make([]AdminVideoListItem, 0, limit+1)

	query := r.db.WithContext(ctx).
		Table("videos").
		Select(`videos.id AS video_id,
			videos.user_id AS author_id,
			users.name AS author_name,
			users.status AS author_status,
			videos.title,
			videos.description,
			videos.category,
			videos.processing_status,
			videos.publish_status,
			videos.created_at,
			videos.updated_at`).
		Joins("JOIN users ON users.id = videos.user_id").
		Where("videos.deleted_at IS NULL")

	if cursor != nil {
		query = query.Where(
			"videos.created_at < ? OR (videos.created_at = ? AND videos.id < ?)",
			cursor.CreatedAt.UTC(),
			cursor.CreatedAt.UTC(),
			cursor.ID,
		)
	}

	if err := query.
		Order("videos.created_at DESC").
		Order("videos.id DESC").
		Limit(limit + 1).
		Scan(&items).Error; err != nil {
		return AdminVideoListResult{}, fmt.Errorf("list admin videos: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	for index := range items {
		items[index].CreatedAt = items[index].CreatedAt.UTC()
		items[index].UpdatedAt = items[index].UpdatedAt.UTC()
	}

	result := AdminVideoListResult{
		Items:   items,
		HasMore: hasMore,
	}

	if len(items) > 0 {
		last := items[len(items)-1]
		result.LastCreatedAt = last.CreatedAt
		result.LastID = last.VideoID
	}

	return result, nil
}

func (r *adminVideoRepository) FindByID(ctx context.Context, videoID uint64) (*AdminVideoDetail, error) {
	if videoID == 0 {
		return nil, entity.ErrInvalidInput
	}

	var detail AdminVideoDetail

	if err := r.db.WithContext(ctx).
		Table("videos").
		Select(`videos.id AS video_id,
			videos.user_id AS author_id,
			users.name AS author_name,
			users.status AS author_status,
			videos.title,
			videos.description,
			videos.category,
			videos.processing_status,
			videos.publish_status,
			videos.created_at,
			videos.updated_at`).
		Joins("JOIN users ON users.id = videos.user_id").
		Where("videos.id = ? AND videos.deleted_at IS NULL", videoID).
		Take(&detail).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrVideoNotFound
		}
		return nil, fmt.Errorf("find admin video detail: %w", err)
	}

	var output AdminVideoOutputMeta

	if err := r.db.WithContext(ctx).
		Table("video_output_metas").
		Select("video_object_key", "thumbnail_object_key").
		Where("video_id = ?", videoID).
		Take(&output).Error; err == nil {
		detail.OutputMeta = &output
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find admin video output meta: %w", err)
	}

	detail.CreatedAt = detail.CreatedAt.UTC()
	detail.UpdatedAt = detail.UpdatedAt.UTC()

	return &detail, nil
}

func (r *adminVideoRepository) Hide(ctx context.Context, adminUserID uint64, videoID uint64, reason string, requestID string, now time.Time) (*AdminVideoState, error) {
	if adminUserID == 0 || videoID == 0 || now.IsZero() {
		return nil, entity.ErrInvalidInput
	}

	var output AdminVideoState

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", videoID).
			Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock admin video for hide: %w", err)
		}

		beforeStatus := video.PublishStatus

		if err := video.HideByAdmin(now); err != nil {
			return err
		}

		audit, err := entity.NewAdminAuditLog(
			adminUserID,
			entity.AdminAuditTargetVideo,
			video.ID,
			entity.AdminAuditActionVideoHide,
			string(beforeStatus),
			string(video.PublishStatus),
			reason,
			requestID,
			now,
		)
		if err != nil {
			return err
		}

		result := tx.Model(&entity.Video{}).
			Where(
				"id = ? AND processing_status = ? AND publish_status = ? AND deleted_at IS NULL",
				video.ID,
				entity.VideoProcessingReady,
				beforeStatus,
			).
			Select("publish_status", "updated_at").
			UpdateColumns(&video)

		if result.Error != nil {
			return fmt.Errorf("save admin hidden video: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return entity.ErrVideoStateConflict
		}

		if err := tx.Create(audit).Error; err != nil {
			return fmt.Errorf("create admin video hide audit log: %w", err)
		}

		output = AdminVideoState{
			VideoID:          video.ID,
			ProcessingStatus: video.ProcessingStatus,
			PublishStatus:    video.PublishStatus,
			UpdatedAt:        video.UpdatedAt.UTC(),
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &output, nil
}

func (r *adminVideoRepository) Restore(ctx context.Context, adminUserID uint64, videoID uint64, reason string, requestID string, now time.Time) (*AdminVideoState, error) {
	if adminUserID == 0 || videoID == 0 || now.IsZero() {
		return nil, entity.ErrInvalidInput
	}

	var output AdminVideoState

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", videoID).
			Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock admin video for restore: %w", err)
		}

		var owner entity.User

		// User行をロックするとUser停止処理と逆順ロックになり得るため、現在Statusだけを読み取る。
		if err := tx.Select("id", "status").
			Where("id = ?", video.UserID).
			Take(&owner).Error; err != nil {
			return fmt.Errorf("find admin video owner for restore: %w", err)
		}

		beforeStatus := video.PublishStatus

		if err := video.RestoreByAdmin(owner.IsActive(), now); err != nil {
			return err
		}

		audit, err := entity.NewAdminAuditLog(
			adminUserID,
			entity.AdminAuditTargetVideo,
			video.ID,
			entity.AdminAuditActionVideoRestore,
			string(beforeStatus),
			string(video.PublishStatus),
			reason,
			requestID,
			now,
		)
		if err != nil {
			return err
		}

		result := tx.Model(&entity.Video{}).
			Where(
				"id = ? AND processing_status = ? AND publish_status = ? AND deleted_at IS NULL",
				video.ID,
				entity.VideoProcessingReady,
				beforeStatus,
			).
			Select("publish_status", "updated_at").
			UpdateColumns(&video)

		if result.Error != nil {
			return fmt.Errorf("save admin restored video: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return entity.ErrVideoStateConflict
		}

		if err := tx.Create(audit).Error; err != nil {
			return fmt.Errorf("create admin video restore audit log: %w", err)
		}

		output = AdminVideoState{
			VideoID:          video.ID,
			ProcessingStatus: video.ProcessingStatus,
			PublishStatus:    video.PublishStatus,
			UpdatedAt:        video.UpdatedAt.UTC(),
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &output, nil
}
