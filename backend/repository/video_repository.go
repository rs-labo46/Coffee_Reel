package repository

import (
	"coffee-reel/entity"
	"context"
	"time"

	"gorm.io/gorm"
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
