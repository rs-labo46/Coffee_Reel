package repository

import (
	"coffee-reel/entity"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IVideoRepository interface {
	CreateWithIdempotency(ctx context.Context, video *entity.Video, record *entity.IdempotencyRecord) (VideoCreateResult, error)
	CompleteUpload(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error)
	FindPublicByID(ctx context.Context, videoID uint64, viewerUserID *uint64) (*PublicVideoItem, error)
	ListPublic(ctx context.Context, input PublicVideoListInput) (PublicVideoPage, error)
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
	LikeCount          int64
	IsLiked            bool
	IsSaved            bool
	Similarity         float32
}

type PublicVideoPage struct {
	Items          []PublicVideoItem
	HasMore        bool
	LastCreatedAt  time.Time
	LastID         uint64
	LastSimilarity float32
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

type PublicVideoSearchMode string

const (
	PublicVideoSearchAll     PublicVideoSearchMode = "all"
	PublicVideoSearchMatched PublicVideoSearchMode = "matched"
	PublicVideoSearchSimilar PublicVideoSearchMode = "similar"
)

type PublicVideoCursor struct {
	ResultType PublicVideoSearchMode
	Similarity float32
	CreatedAt  time.Time
	ID         uint64
	FilterHash string
}

type PublicVideoListInput struct {
	Title        string
	Category     *entity.CategoryCode
	AuthorID     *uint64
	Limit        int
	Cursor       *PublicVideoCursor
	ViewerUserID *uint64
	SearchMode   PublicVideoSearchMode
}

type videoRepository struct {
	db *gorm.DB
}

func NewVideoRepository(db *gorm.DB) IVideoRepository {
	return &videoRepository{db}
}

func (m PublicVideoSearchMode) IsValid() bool {
	switch m {
	case PublicVideoSearchAll, PublicVideoSearchMatched, PublicVideoSearchSimilar:
		return true
	default:
		return false
	}
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
	if videoID == 0 || (viewerUserID != nil && *viewerUserID == 0) {
		return nil, entity.ErrInvalidInput
	}

	viewerID := uint64(0)
	if viewerUserID != nil {
		viewerID = *viewerUserID
	}

	var item PublicVideoItem
	err := r.publicQuery(ctx, viewerID).
		Where("videos.id = ?", videoID).
		Take(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrVideoNotFound
		}
		return nil, fmt.Errorf("find public video: %w", err)
	}

	return &item, nil
}

func (r *videoRepository) ListPublic(ctx context.Context, input PublicVideoListInput) (PublicVideoPage, error) {
	if err := validatePublicVideoListInput(input); err != nil {
		return PublicVideoPage{}, err
	}

	viewerID := uint64(0)
	if input.ViewerUserID != nil {
		viewerID = *input.ViewerUserID
	}

	items := make([]PublicVideoItem, 0, input.Limit+1)
	var query *gorm.DB

	if input.SearchMode == PublicVideoSearchSimilar {
		query = r.publicSimilarQuery(ctx, viewerID, input.Title).
			Where("lower(?) <% lower(videos.title)", input.Title)
	} else {
		query = r.publicQuery(ctx, viewerID)
	}

	if input.Category != nil {
		query = query.Where("videos.category = ?", *input.Category)
	}

	if input.AuthorID != nil {
		query = query.Where("videos.user_id = ?", *input.AuthorID)
	}

	if input.SearchMode == PublicVideoSearchMatched && input.Title != "" {
		query = query.Where(
			"LOWER(videos.title) LIKE ? ESCAPE '\\'",
			publicTitlePattern(input.Title),
		)
	}

	if input.Cursor != nil {
		if input.SearchMode == PublicVideoSearchSimilar {
			query = query.Where(
				"(word_similarity(lower(?), lower(videos.title)), videos.created_at, videos.id) < (?, ?, ?)",
				input.Title,
				input.Cursor.Similarity,
				input.Cursor.CreatedAt,
				input.Cursor.ID,
			)
		} else {
			query = query.Where(
				"videos.created_at < ? OR (videos.created_at = ? AND videos.id < ?)",
				input.Cursor.CreatedAt,
				input.Cursor.CreatedAt,
				input.Cursor.ID,
			)
		}
	}

	if input.SearchMode == PublicVideoSearchSimilar {
		query = query.Order("similarity DESC")
	}

	if err := query.
		Order("videos.created_at DESC").
		Order("videos.id DESC").
		Limit(input.Limit + 1).
		Scan(&items).Error; err != nil {
		return PublicVideoPage{}, fmt.Errorf("list public videos: %w", err)
	}

	hasMore := len(items) > input.Limit
	if hasMore {
		items = items[:input.Limit]
	}

	page := PublicVideoPage{
		Items:   items,
		HasMore: hasMore,
	}
	if len(items) > 0 {
		last := items[len(items)-1]
		page.LastCreatedAt = last.CreatedAt
		page.LastID = last.ID
		page.LastSimilarity = last.Similarity
	}

	return page, nil
}

func validatePublicVideoListInput(input PublicVideoListInput) error {
	if input.Limit < 1 || input.Limit > 100 || !input.SearchMode.IsValid() {
		return entity.ErrInvalidInput
	}
	if input.ViewerUserID != nil && *input.ViewerUserID == 0 {
		return entity.ErrInvalidInput
	}
	if input.Category != nil && !input.Category.IsValid() {
		return entity.ErrInvalidInput
	}
	if input.AuthorID != nil && *input.AuthorID == 0 {
		return entity.ErrInvalidInput
	}

	switch input.SearchMode {
	case PublicVideoSearchAll:
		if input.Title != "" || input.Category != nil {
			return entity.ErrInvalidInput
		}
	case PublicVideoSearchMatched:
		if input.Title == "" && input.Category == nil {
			return entity.ErrInvalidInput
		}
	case PublicVideoSearchSimilar:
		if input.Title == "" {
			return entity.ErrInvalidInput
		}
	}

	if input.Cursor == nil {
		return nil
	}
	if input.Cursor.ResultType != input.SearchMode ||
		input.Cursor.CreatedAt.IsZero() ||
		input.Cursor.ID == 0 ||
		strings.TrimSpace(input.Cursor.FilterHash) == "" {
		return entity.ErrCursorInvalid
	}
	if input.SearchMode == PublicVideoSearchSimilar {
		if input.Cursor.Similarity < 0.6 || input.Cursor.Similarity > 1 {
			return entity.ErrCursorInvalid
		}
	} else if input.Cursor.Similarity != 0 {
		return entity.ErrCursorInvalid
	}

	return nil
}

func (r *videoRepository) publicQuery(ctx context.Context, viewerID uint64) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos").
		Select(`videos.id, videos.user_id, users.name AS author_name, videos.category, videos.title, videos.description,
			video_output_metas.video_object_key, video_output_metas.thumbnail_object_key, videos.created_at,
			COALESCE(video_like_counts.like_count, 0) AS like_count,
			CASE WHEN ? = 0 THEN FALSE ELSE EXISTS (
				SELECT 1 FROM video_likes WHERE video_likes.user_id = ? AND video_likes.video_id = videos.id
			) END AS is_liked,
			CASE WHEN ? = 0 THEN FALSE ELSE EXISTS (
				SELECT 1 FROM saved_videos WHERE saved_videos.user_id = ? AND saved_videos.video_id = videos.id
			) END AS is_saved`, viewerID, viewerID, viewerID, viewerID).
		Joins("JOIN users ON users.id = videos.user_id").
		Joins("JOIN video_output_metas ON video_output_metas.video_id = videos.id").
		Joins(`LEFT JOIN (
			SELECT video_id, COUNT(*) AS like_count
			FROM video_likes
			GROUP BY video_id
		) AS video_like_counts ON video_like_counts.video_id = videos.id`).
		Where(
			"videos.processing_status = ? AND videos.publish_status = ? AND videos.deleted_at IS NULL",
			entity.VideoProcessingReady,
			entity.VideoPublishPublished,
		)
}

func (r *videoRepository) publicSimilarQuery(ctx context.Context, viewerID uint64, title string) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos").
		Select(`videos.id, videos.user_id, users.name AS author_name, videos.category, videos.title, videos.description,
			video_output_metas.video_object_key, video_output_metas.thumbnail_object_key, videos.created_at,
			COALESCE(video_like_counts.like_count, 0) AS like_count,
			CASE WHEN ? = 0 THEN FALSE ELSE EXISTS (
				SELECT 1 FROM video_likes WHERE video_likes.user_id = ? AND video_likes.video_id = videos.id
			) END AS is_liked,
			CASE WHEN ? = 0 THEN FALSE ELSE EXISTS (
				SELECT 1 FROM saved_videos WHERE saved_videos.user_id = ? AND saved_videos.video_id = videos.id
			) END AS is_saved,
			word_similarity(lower(?), lower(videos.title)) AS similarity`, viewerID, viewerID, viewerID, viewerID, title).
		Joins("JOIN users ON users.id = videos.user_id").
		Joins("JOIN video_output_metas ON video_output_metas.video_id = videos.id").
		Joins(`LEFT JOIN (
			SELECT video_id, COUNT(*) AS like_count
			FROM video_likes
			GROUP BY video_id
		) AS video_like_counts ON video_like_counts.video_id = videos.id`).
		Where(
			"videos.processing_status = ? AND videos.publish_status = ? AND videos.deleted_at IS NULL",
			entity.VideoProcessingReady,
			entity.VideoPublishPublished,
		)
}

func publicTitlePattern(title string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	).Replace(strings.ToLower(title))
	return "%" + escaped + "%"
}

func (r *videoRepository) FindOwnedByID(ctx context.Context, userID, videoID uint64) (*OwnedVideoDetail, error) {
	var video entity.Video
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ? AND deleted_at IS NULL", videoID, userID).Take(&video).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrVideoNotFound
		}
		return nil, fmt.Errorf("find owned video: %w", err)
	}
	detail := &OwnedVideoDetail{Video: &video}
	var source entity.SourceVideoMeta
	if err := r.db.WithContext(ctx).Where("video_id = ?", video.ID).Take(&source).Error; err == nil {
		detail.SourceMeta = &source
		video.SourceMeta = &source
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find source meta: %w", err)
	}
	var output entity.OutputVideoMeta
	if err := r.db.WithContext(ctx).Where("video_id = ?", video.ID).Take(&output).Error; err == nil {
		detail.OutputMeta = &output
		video.OutputMeta = &output
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find output meta: %w", err)
	}
	if video.ProcessingStatus == entity.VideoProcessingFailed {
		var job entity.VideoProcessingJob
		if err := r.db.WithContext(ctx).Where("video_id = ?", video.ID).Take(&job).Error; err == nil && job.LastErrorCode != "" {
			code := publicFailureCode(job.LastErrorCode)
			detail.FailureCode = &code
		}
	}
	return detail, nil
}
func publicFailureCode(code string) entity.VideoFailureCode {
	switch entity.VideoFailureCode(code) {
	case entity.VideoFailureInvalidFormat,
		entity.VideoFailureCorrupt,
		entity.VideoFailureDurationExceeded,
		entity.VideoFailureSizeExceeded,
		entity.VideoFailureResolutionExceeded,
		entity.VideoFailureInvalidAspectRatio,
		entity.VideoFailureFrameRateExceeded,
		entity.VideoFailureVideoTrackMissing:
		return entity.VideoFailureCode(code)
	default:
		return entity.VideoFailureProcessingFailed
	}
}

func (r *videoRepository) ListOwned(
	ctx context.Context,
	userID uint64,
	limit int,
	cursor *VideoCursor,
) (OwnedVideoPage, error) {
	if userID == 0 ||
		limit < 1 ||
		limit > 100 ||
		(cursor != nil && (cursor.CreatedAt.IsZero() || cursor.ID == 0)) {
		return OwnedVideoPage{}, entity.ErrInvalidInput
	}

	items := make([]OwnedVideoItem, 0, limit+1)

	query := r.db.WithContext(ctx).
		Table("videos").
		Select(`videos.id,
			videos.category,
			videos.title,
			videos.description,
			videos.processing_status,
			videos.publish_status,
			videos.upload_expires_at,
			videos.created_at,
			videos.updated_at,
			COALESCE(video_output_metas.thumbnail_object_key, '') AS thumbnail_object_key`).
		Joins("LEFT JOIN video_output_metas ON video_output_metas.video_id = videos.id").
		Where("videos.user_id = ? AND videos.deleted_at IS NULL", userID)

	if cursor != nil {
		query = query.Where(
			"(videos.created_at < ? OR (videos.created_at = ? AND videos.id < ?))",
			cursor.CreatedAt,
			cursor.CreatedAt,
			cursor.ID,
		)
	}

	if err := query.
		Order("videos.created_at DESC").
		Order("videos.id DESC").
		Limit(limit + 1).
		Scan(&items).Error; err != nil {
		return OwnedVideoPage{}, fmt.Errorf("list owned videos: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	page := OwnedVideoPage{
		Items:   items,
		HasMore: hasMore,
	}

	if len(items) > 0 {
		last := items[len(items)-1]
		page.LastCreatedAt = last.CreatedAt
		page.LastID = last.ID
	}

	return page, nil
}
func (r *videoRepository) SetPrivateByOwner(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error) {
	return r.updateOwnedVideo(ctx, userID, videoID, func(video *entity.Video) error {
		return video.SetPrivateByOwner(userID, now)
	}, []string{"publish_status", "updated_at"})
}

func (r *videoRepository) updateOwnedVideo(ctx context.Context, userID, videoID uint64, change func(*entity.Video) error, fields []string) (*entity.Video, error) {
	var output entity.Video
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", videoID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock owned video: %w", err)
		}
		if err := change(&video); err != nil {
			return err
		}
		if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select(fields).Updates(&video).Error; err != nil {
			return fmt.Errorf("save owned video: %w", err)
		}
		output = video
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &output, nil
}

func (r *videoRepository) RepublishByOwner(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error) {
	var output entity.Video
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ?", userID).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrUserNotFound
			}
			return fmt.Errorf("lock owner for republish: %w", err)
		}
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", videoID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock video for republish: %w", err)
		}
		if err := video.RepublishByOwner(userID, user.IsActive(), now); err != nil {
			return err
		}
		if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select("publish_status", "updated_at").Updates(&video).Error; err != nil {
			return fmt.Errorf("save republished video: %w", err)
		}
		output = video
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &output, nil
}

func (r *videoRepository) DeleteByOwner(ctx context.Context, userID, videoID uint64, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", videoID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock video for deletion: %w", err)
		}
		if err := video.DeleteByOwner(userID, now); err != nil {
			return err
		}
		if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).
			Select("publish_status", "deleted_at", "updated_at").Updates(&video).Error; err != nil {
			return fmt.Errorf("save deleted video: %w", err)
		}
		if err := tx.Model(&entity.VideoProcessingJob{}).
			Where("video_id = ? AND status IN ?", video.ID, []entity.VideoProcessingJobStatus{entity.VideoJobQueued, entity.VideoJobRetryWait}).
			Updates(entity.VideoProcessingJob{Status: entity.VideoJobCancelled, FinishedAt: ptrTime(now), UpdatedAt: now}).Error; err != nil {
			return fmt.Errorf("cancel processing job: %w", err)
		}
		if err := createCleanupJob(tx, &video.ID, video.OriginalObjectKey, entity.StorageAssetOriginal, entity.StorageCleanupVideoDelete, now); err != nil {
			return err
		}
		var output entity.OutputVideoMeta
		if err := tx.Where("video_id = ?", video.ID).Take(&output).Error; err == nil {
			if err := createCleanupJob(tx, &video.ID, output.VideoObjectKey, entity.StorageAssetProcessed, entity.StorageCleanupVideoDelete, now); err != nil {
				return err
			}
			if err := createCleanupJob(tx, &video.ID, output.ThumbnailObjectKey, entity.StorageAssetThumbnail, entity.StorageCleanupVideoDelete, now); err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find output meta for deletion: %w", err)
		}
		return nil
	})
}

func createCleanupJob(tx *gorm.DB, videoID *uint64, objectKey string, assetType entity.StorageAssetType, cause entity.StorageCleanupCause, now time.Time) error {
	if strings.TrimSpace(objectKey) == "" {
		return nil
	}
	job, err := entity.NewStorageCleanupJob(videoID, objectKey, assetType, cause, now)
	if err != nil {
		return err
	}
	if err := tx.Create(job).Error; err != nil {
		if isConstraintViolation(err, "uq_storage_cleanup_jobs_unfinished_object") {
			return nil
		}
		return fmt.Errorf("create storage cleanup job: %w", err)
	}
	return nil
}

func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation && (constraint == "" || pgErr.ConstraintName == constraint)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func (r *videoRepository) ExpireUploads(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 {
		return 0, entity.ErrInvalidInput
	}
	count := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var videos []entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("processing_status = ? AND publish_status = ? AND deleted_at IS NULL AND upload_expires_at <= ?", entity.VideoProcessingUploading, entity.VideoPublishPrivate, now).
			Order("upload_expires_at ASC").Order("id ASC").Limit(limit).Find(&videos).Error; err != nil {
			return fmt.Errorf("claim expired uploads: %w", err)
		}
		for i := range videos {
			video := &videos[i]
			if err := video.ExpireUpload(now); err != nil {
				return err
			}
			if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select("processing_status", "updated_at").Updates(video).Error; err != nil {
				return fmt.Errorf("save expired upload: %w", err)
			}
			if err := createCleanupJob(tx, &video.ID, video.OriginalObjectKey, entity.StorageAssetOriginal, entity.StorageCleanupUploadExpired, now); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (r *videoRepository) RecordSourceValidation(ctx context.Context, videoID uint64, meta entity.SourceVideoMeta, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", videoID).Take(&video).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrVideoNotFound
			}
			return fmt.Errorf("lock video for source validation: %w", err)
		}
		var existing int64
		if err := tx.Model(&entity.SourceVideoMeta{}).Where("video_id = ?", videoID).Count(&existing).Error; err != nil {
			return fmt.Errorf("check source meta: %w", err)
		}
		if existing > 0 {
			return nil
		}
		if err := video.RecordSourceValidation(meta, now); err != nil {
			return err
		}
		if err := tx.Create(video.SourceMeta).Error; err != nil {
			return fmt.Errorf("create source meta: %w", err)
		}
		if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Update("updated_at", video.UpdatedAt).Error; err != nil {
			return fmt.Errorf("update video source validation time: %w", err)
		}
		return nil
	})
}

func (r *videoRepository) CompleteProcessing(ctx context.Context, input ProcessingCompletionInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.VideoProcessingJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", input.JobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrProcessingJobNotFound
			}
			return fmt.Errorf("lock processing job for completion: %w", err)
		}
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", job.VideoID).Take(&video).Error; err != nil {
			return fmt.Errorf("lock video for processing completion: %w", err)
		}
		if video.DeletedAt != nil {
			if err := job.Cancel(input.Now); err != nil && !errors.Is(err, entity.ErrProcessingJobConflict) {
				return err
			}
			if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).
				Select("status", "finished_at", "updated_at").Updates(&job).Error; err != nil {
				return fmt.Errorf("cancel deleted video job: %w", err)
			}
			if err := createCleanupJob(tx, &video.ID, input.OutputMeta.VideoObjectKey, entity.StorageAssetProcessed, entity.StorageCleanupRollback, input.Now); err != nil {
				return err
			}
			if err := createCleanupJob(tx, &video.ID, input.OutputMeta.ThumbnailObjectKey, entity.StorageAssetThumbnail, entity.StorageCleanupRollback, input.Now); err != nil {
				return err
			}
			return nil
		}
		var source entity.SourceVideoMeta
		if err := tx.Where("video_id = ?", video.ID).Take(&source).Error; err != nil {
			return fmt.Errorf("find source meta for completion: %w", err)
		}
		video.SourceMeta = &source
		var owner entity.User
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ?", video.UserID).Take(&owner).Error; err != nil {
			return fmt.Errorf("lock video owner for completion: %w", err)
		}
		if err := video.CompleteProcessing(input.OutputMeta, owner.IsActive(), input.Now); err != nil {
			return err
		}
		if err := job.MarkSucceeded(input.Now); err != nil {
			return err
		}
		if err := tx.Create(video.OutputMeta).Error; err != nil {
			return fmt.Errorf("create output meta: %w", err)
		}
		if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).
			Select("processing_status", "publish_status", "updated_at").Updates(&video).Error; err != nil {
			return fmt.Errorf("save completed video: %w", err)
		}
		if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).
			Select("status", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save succeeded processing job: %w", err)
		}
		return nil
	})
}
func (r *videoRepository) FailProcessing(ctx context.Context, input ProcessingFailureInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job entity.VideoProcessingJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", input.JobID).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrProcessingJobNotFound
			}
			return fmt.Errorf("lock processing job for failure: %w", err)
		}
		var video entity.Video
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", job.VideoID).Take(&video).Error; err != nil {
			return fmt.Errorf("lock video for processing failure: %w", err)
		}
		if video.DeletedAt != nil {
			if err := job.Cancel(input.Now); err != nil {
				return err
			}
		} else {
			if err := video.FailProcessing(input.Now); err != nil {
				return err
			}
			if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Select("processing_status", "publish_status", "updated_at").Updates(&video).Error; err != nil {
				return fmt.Errorf("save failed video: %w", err)
			}
			if err := job.MarkFailed(string(input.FailureCode), safeErrorMessage(input.FailureMessage), input.Now); err != nil {
				return err
			}
		}
		if err := tx.Model(&entity.VideoProcessingJob{}).Where("id = ?", job.ID).
			Select("status", "finished_at", "last_error_code", "last_error_message", "updated_at").Updates(&job).Error; err != nil {
			return fmt.Errorf("save failed processing job: %w", err)
		}

		if input.GeneratedVideoKey != "" {
			if err := createCleanupJob(
				tx,
				&video.ID,
				input.GeneratedVideoKey,
				entity.StorageAssetProcessed,
				entity.StorageCleanupProcessFailed,
				input.Now,
			); err != nil {
				return err
			}
		}

		if input.GeneratedThumbnailKey != "" {
			if err := createCleanupJob(
				tx,
				&video.ID,
				input.GeneratedThumbnailKey,
				entity.StorageAssetThumbnail,
				entity.StorageCleanupProcessFailed,
				input.Now,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func safeErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func (r *videoRepository) IsObjectReferenced(ctx context.Context, objectKey string) (bool, error) {
	if strings.TrimSpace(objectKey) == "" {
		return false, entity.ErrInvalidInput
	}
	var count int64
	if err := r.db.WithContext(ctx).Table("videos").Where("original_object_key = ?", objectKey).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check original object reference: %w", err)
	}
	if count > 0 {
		return true, nil
	}
	if err := r.db.WithContext(ctx).Table("video_output_metas").Where("video_object_key = ? OR thumbnail_object_key = ?", objectKey, objectKey).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check output object reference: %w", err)
	}
	return count > 0, nil
}
func (r *videoRepository) DeleteExpiredIdempotencyRecords(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit < 1 {
		return 0, entity.ErrInvalidInput
	}
	var ids []uint64
	if err := r.db.WithContext(ctx).Model(&entity.IdempotencyRecord{}).Where("expires_at <= ?", now).Order("expires_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("list expired idempotency records: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("id IN ? AND expires_at <= ?", ids, now).Delete(&entity.IdempotencyRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired idempotency records: %w", result.Error)
	}
	return result.RowsAffected, nil
}
