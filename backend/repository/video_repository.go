package repository

import (
	"coffee-reel/entity"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IVideoRepository interface {
	CreateWithIdempotency(ctx context.Context, video *entity.Video, record *entity.IdempotencyRecord) (VideoCreateResult, error)
	CompleteUpload(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error)
	FindPublicByID(ctx context.Context, videoID uint64, viewerUserID *uint64) (*PublicVideoItem, error)
	ListPublic(ctx context.Context, limit int, cursor *VideoCursor, viewerUserID *uint64) (PublicVideoPage, error)
	FindOwnedByID(ctx context.Context, userID, videoID uint64) (*OwnedVideoDetail, error)
	ListOwned(ctx context.Context, userID uint64, limit int, cursor *VideoCursor) (OwnedVideoPage, error)
	SetPrivateByOwner(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error)
	RepublishByOwner(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error)
	DeleteByOwner(ctx context.Context, userID, videoID uint64, now time.Time) error
	ExpireUploads(ctx context.Context, now time.Time, limit int) (int, error)
	RecordSourceValidation(ctx context.Context, videoID uint64, meta entity.SourceVideoMeta, now time.Time) error
	CompleteProcessing(ctx context.Context, input ProcessingCompletionInput) error
	FailProcessing(ctx context.Context, input ProcessingFailureInput) error
	IsObjectReferenced(ctx context.Context, objectKey string) (bool, error)
	DeleteExpiredIdempotencyRecords(ctx context.Context, now time.Time, limit int) (int64, error)
}

type VideoCreateResult struct {
	Video   *entity.Video
	Created bool
}

type VideoCursor struct {
	CreatedAt time.Time
	ID        uint64
}

type PublicVideoItem struct {
	ID                 uint64
	UserID             uint64
	AuthorName         string
	Category           entity.CategoryCode
	Title              string
	Description        string
	VideoObjectKey     string
	ThumbnailObjectKey string
	CreatedAt          time.Time
	IsSaved            bool
}

type PublicVideoPage struct {
	Items         []PublicVideoItem
	HasMore       bool
	LastCreatedAt time.Time
	LastID        uint64
}

type OwnedVideoItem struct {
	ID                 uint64
	Category           entity.CategoryCode
	Title              string
	Description        string
	ProcessingStatus   entity.VideoProcessingStatus
	PublishStatus      entity.VideoPublishStatus
	UploadExpiresAt    time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ThumbnailObjectKey string
}

type OwnedVideoPage struct {
	Items         []OwnedVideoItem
	HasMore       bool
	LastCreatedAt time.Time
	LastID        uint64
}

type OwnedVideoDetail struct {
	Video       *entity.Video
	SourceMeta  *entity.SourceVideoMeta
	OutputMeta  *entity.OutputVideoMeta
	FailureCode *entity.VideoFailureCode
}

type ProcessingCompletionInput struct {
	JobID      uint64
	OutputMeta entity.OutputVideoMeta
	Now        time.Time
}

type ProcessingFailureInput struct {
	JobID                 uint64
	FailureCode           entity.VideoFailureCode
	FailureMessage        string
	GeneratedVideoKey     string
	GeneratedThumbnailKey string
	Now                   time.Time
}

type videoRepository struct {
	db *gorm.DB
}

func NewVideoRepository(db *gorm.DB) IVideoRepository {
	return &videoRepository{db}
}

func (r *videoRepository) CreateWithIdempotency(ctx context.Context, video *entity.Video, record *entity.IdempotencyRecord) (VideoCreateResult, error) {
	if video == nil ||
		record == nil ||
		video.ID != 0 ||
		record.ID != 0 ||
		record.ResourceID != 0 ||
		video.UserID == 0 ||
		video.UserID != record.UserID ||
		!record.Scope.IsValid() ||
		strings.TrimSpace(video.OriginalObjectKey) == "" {
		return VideoCreateResult{}, entity.ErrInvalidInput
	}
	var result VideoCreateResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing entity.IdempotencyRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND scope = ? AND key_hash = ?", record.UserID, record.Scope, record.KeyHash).
			Take(&existing).Error
		if err == nil {
			return loadIdempotentVideo(tx, existing, record.RequestHash, &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find idempotency record: %w", err)
		}

		if err := tx.Create(video).Error; err != nil {
			return fmt.Errorf("create video: %w", err)
		}
		record.ResourceID = video.ID
		if err := tx.Create(record).Error; err != nil {
			return fmt.Errorf("create idempotency record: %w", err)
		}
		result = VideoCreateResult{Video: video, Created: true}
		return nil
	})
	if err == nil {
		return result, nil
	}
	if errors.Is(err, entity.ErrIdempotencyConflict) {
		return VideoCreateResult{}, err
	}
	if !isConstraintViolation(err, "uq_idempotency_records_user_scope_key") {
		return VideoCreateResult{}, fmt.Errorf("create video with idempotency: %w", err)
	}

	var existing entity.IdempotencyRecord
	if findErr := r.db.WithContext(ctx).
		Where("user_id = ? AND scope = ? AND key_hash = ?", record.UserID, record.Scope, record.KeyHash).
		Take(&existing).Error; findErr != nil {
		return VideoCreateResult{}, fmt.Errorf("find concurrent idempotency record: %w", findErr)
	}
	if err := loadIdempotentVideo(r.db.WithContext(ctx), existing, record.RequestHash, &result); err != nil {
		return VideoCreateResult{}, err
	}
	return result, nil
}

func loadIdempotentVideo(db *gorm.DB, record entity.IdempotencyRecord, requestHash string, result *VideoCreateResult) error {
	if record.RequestHash != requestHash {
		return entity.ErrIdempotencyConflict
	}
	var video entity.Video
	if err := db.Where("id = ?", record.ResourceID).Take(&video).Error; err != nil {
		return fmt.Errorf("find idempotent video: %w", err)
	}
	*result = VideoCreateResult{Video: &video, Created: false}
	return nil
}

func (r *videoRepository) CompleteUpload(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error) {
	if userID == 0 || videoID == 0 || now.IsZero() {
		return nil, entity.ErrInvalidInput
	}

	var output entity.Video
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND deleted_at IS NULL", videoID, userID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock video for upload completion: %w", err)
		}

		switch video.ProcessingStatus {
		case entity.VideoProcessingUploading:
			if err := video.CompleteUpload(now); err != nil {
				return err
			}
			if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select("processing_status", "updated_at").Updates(&video).Error; err != nil {
				return fmt.Errorf("save completed upload: %w", err)
			}
			job, err := entity.NewVideoProcessingJob(video.ID, now)
			if err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "video_id"}},
				DoNothing: true,
			}).Create(job).Error; err != nil {
				return fmt.Errorf("create processing job: %w", err)
			}
		case entity.VideoProcessingExpired:
			return entity.ErrUploadExpired
		case entity.VideoProcessingUploaded,
			entity.VideoProcessingProcessing,
			entity.VideoProcessingReady,
			entity.VideoProcessingFailed:
			// 完了通知の再送では状態を変更せず、現在のVideoを返す。
		default:
			return entity.ErrVideoStateConflict
		}

		output = video
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &output, nil
}
func (r *videoRepository) FindPublicByID(ctx context.Context, videoID uint64, viewerUserID *uint64) (*PublicVideoItem, error) {
	viewerID := uint64(0)
	if viewerUserID != nil {
		viewerID = *viewerUserID
	}
	var item PublicVideoItem
	err := r.publicQuery(ctx, viewerID).Where("videos.id = ?", videoID).Take(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrVideoNotFound
		}
		return nil, fmt.Errorf("find public video: %w", err)
	}
	return &item, nil
}

func (r *videoRepository) publicQuery(ctx context.Context, viewerID uint64) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos").
		Select(`videos.id, videos.user_id, users.name AS author_name, videos.category, videos.title, videos.description,
			video_output_metas.video_object_key, video_output_metas.thumbnail_object_key, videos.created_at,
			CASE WHEN ? = 0 THEN FALSE ELSE EXISTS (
				SELECT 1 FROM saved_videos WHERE saved_videos.user_id = ? AND saved_videos.video_id = videos.id
			) END AS is_saved`, viewerID, viewerID).
		Joins("JOIN users ON users.id = videos.user_id").
		Joins("JOIN video_output_metas ON video_output_metas.video_id = videos.id").
		Where("videos.processing_status = ? AND videos.publish_status = ? AND videos.deleted_at IS NULL", entity.VideoProcessingReady, entity.VideoPublishPublished)
}
