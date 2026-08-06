package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type IVideoUsecase interface {
	StartUpload(ctx context.Context, actor *entity.User, input StartUploadInput, idempotencyKey string) (StartUploadResult, error)
	CompleteUpload(ctx context.Context, actor *entity.User, videoID uint64) (VideoStateResult, error)
	ListReels(ctx context.Context, viewer *entity.User, input PublicVideoListInput) (PublicVideoListResult, error)
	GetDetail(ctx context.Context, viewer *entity.User, videoID uint64) (PublicVideoResult, error)
	ListMine(ctx context.Context, actor *entity.User, input VideoListInput) (OwnedVideoListResult, error)
	GetMine(ctx context.Context, actor *entity.User, videoID uint64) (OwnedVideoDetailResult, error)
	SetPrivate(ctx context.Context, actor *entity.User, videoID uint64) (VideoStateResult, error)
	Republish(ctx context.Context, actor *entity.User, videoID uint64) (VideoStateResult, error)
	Delete(ctx context.Context, actor *entity.User, videoID uint64) error
}

const (
	videoObjectRandomBytes = 16
	maxVideoSizeBytes      = 30_000_000
)

type VideoUsecaseConfig struct {
	UploadURLTTL       time.Duration
	ReadURLTTL         time.Duration
	IdempotencyTTL     time.Duration
	IdempotencyHMACKey []byte
	ManagedPrefix      string
}

type StartUploadInput struct {
	Title        string
	Description  string
	Category     entity.CategoryCode
	ContentType  string
	DeclaredSize int64
}

type UploadInfo struct {
	Method      string
	URL         string
	ContentType string
	ExpiresAt   time.Time
}

type ReadInfo struct {
	URL       string
	ExpiresAt time.Time
}

type StartUploadResult struct {
	VideoID          uint64
	Title            string
	Description      string
	Category         entity.CategoryCode
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	UploadExpiresAt  time.Time
	CreatedAt        time.Time
	Upload           UploadInfo
	Created          bool
}

type VideoStateResult struct {
	ID               uint64
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	UpdatedAt        time.Time
}

type VideoCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uint64    `json:"id"`
}

type VideoListInput struct {
	Limit  int
	Cursor *VideoCursor
}
type PublicSearchResultType string

const (
	PublicSearchAll     PublicSearchResultType = "all"
	PublicSearchMatched PublicSearchResultType = "matched"
	PublicSearchSimilar PublicSearchResultType = "similar"
)

func (r PublicSearchResultType) IsValid() bool {
	switch r {
	case PublicSearchAll, PublicSearchMatched, PublicSearchSimilar:
		return true
	default:
		return false
	}
}

type PublicVideoCursor struct {
	ResultType PublicSearchResultType `json:"result_type"`
	Similarity float32                `json:"similarity"`
	CreatedAt  time.Time              `json:"created_at"`
	ID         uint64                 `json:"id"`
	FilterHash string                 `json:"filter_hash"`
}

type PublicVideoListInput struct {
	Title    string
	Category *entity.CategoryCode
	Limit    int
	Cursor   *PublicVideoCursor
}

type PublicVideoResult struct {
	ID          uint64
	UserID      uint64
	AuthorName  string
	Category    entity.CategoryCode
	Title       string
	Description string
	CreatedAt   time.Time
	Video       ReadInfo
	Thumbnail   ReadInfo
	LikeCount   int64
	IsLiked     bool
	IsSaved     bool
}

type PublicVideoListResult struct {
	Items      []PublicVideoResult
	ResultType PublicSearchResultType
	NextCursor *string
	HasMore    bool
}

type OwnedVideoResult struct {
	ID               uint64
	Category         entity.CategoryCode
	Title            string
	Description      string
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	UploadExpiresAt  time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Thumbnail        *ReadInfo
}

type OwnedVideoListResult struct {
	Items      []OwnedVideoResult
	NextCursor *string
	HasMore    bool
}

type OwnedVideoDetailResult struct {
	ID               uint64
	Category         entity.CategoryCode
	Title            string
	Description      string
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	UploadExpiresAt  time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	SourceMeta       *SourceVideoMetaResult
	OutputMeta       *OwnedOutputMetaResult
	FailureCode      *entity.VideoFailureCode
}

type SourceVideoMetaResult struct {
	MIMEType       string
	Container      string
	SizeBytes      int64
	DurationMillis int64
	Width          int
	Height         int
	FrameRate      float64
	VideoCodec     string
	HasAudio       bool
	AudioCodec     string
	CreatedAt      time.Time
}

type OwnedOutputMetaResult struct {
	Container  string
	Width      int
	Height     int
	FrameRate  float64
	VideoCodec string
	HasAudio   bool
	AudioCodec string
	Video      ReadInfo
	Thumbnail  ReadInfo
}

type videoUsecase struct {
	videos             repository.IVideoRepository
	storage            repository.IObjectStorageRepository
	uploadURLTTL       time.Duration
	readURLTTL         time.Duration
	idempotencyTTL     time.Duration
	idempotencyHMACKey []byte
	managedPrefix      string
}

func NewVideoUsecase(videos repository.IVideoRepository, storage repository.IObjectStorageRepository, config VideoUsecaseConfig) (IVideoUsecase, error) {
	managedPrefix := normalizeUsecaseManagedPrefix(config.ManagedPrefix)
	if videos == nil || storage == nil || config.UploadURLTTL <= 0 || config.ReadURLTTL <= 0 || config.IdempotencyTTL <= 0 || len(config.IdempotencyHMACKey) == 0 || managedPrefix != "videos/" {
		return nil, fmt.Errorf("video usecase configuration is invalid")
	}

	keyCopy := append([]byte(nil), config.IdempotencyHMACKey...)
	return &videoUsecase{
		videos:             videos,
		storage:            storage,
		uploadURLTTL:       config.UploadURLTTL,
		readURLTTL:         config.ReadURLTTL,
		idempotencyTTL:     config.IdempotencyTTL,
		idempotencyHMACKey: keyCopy,
		managedPrefix:      managedPrefix,
	}, nil
}

func (u *videoUsecase) StartUpload(ctx context.Context, actor *entity.User, input StartUploadInput, idempotencyKey string) (StartUploadResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return StartUploadResult{}, err
	}
	if err := validateStartUploadInput(input, idempotencyKey); err != nil {
		return StartUploadResult{}, err
	}

	now := time.Now().UTC()
	objectKey, err := buildSourceObjectKey(u.managedPrefix, sourceExtension(input.ContentType))
	if err != nil {
		return StartUploadResult{}, err
	}

	uploadExpiresAt := now.Add(u.uploadURLTTL)
	video, err := entity.NewVideo(actor.ID, input.Category, input.Title, input.Description, objectKey, uploadExpiresAt, now)
	if err != nil {
		return StartUploadResult{}, err
	}
	keyHash := hashIdempotencyKey(u.idempotencyHMACKey, idempotencyKey)
	requestHash, err := hashStartUploadRequest(input)
	if err != nil {
		return StartUploadResult{}, err
	}
	record, err := entity.NewIdempotencyRecord(actor.ID, keyHash, requestHash, now.Add(u.idempotencyTTL), now)
	if err != nil {
		return StartUploadResult{}, err
	}

	created, err := u.videos.CreateWithIdempotency(ctx, video, record)
	if err != nil {
		return StartUploadResult{}, err
	}
	if created.Video == nil {
		return StartUploadResult{}, fmt.Errorf("create video returned no video")
	}

	current := created.Video
	if current.DeletedAt != nil {
		return StartUploadResult{}, entity.ErrVideoNotFound
	}
	if current.ProcessingStatus == entity.VideoProcessingExpired {
		return StartUploadResult{}, entity.ErrUploadExpired
	}
	if current.ProcessingStatus != entity.VideoProcessingUploading || current.PublishStatus != entity.VideoPublishPrivate {
		return StartUploadResult{}, entity.ErrVideoStateConflict
	}

	issuedAt := time.Now().UTC()
	remaining := current.UploadExpiresAt.UTC().Sub(issuedAt)
	if remaining <= 0 {
		return StartUploadResult{}, entity.ErrUploadExpired
	}
	uploadTarget, err := u.storage.CreateUploadURL(ctx, current.OriginalObjectKey, input.ContentType, remaining)
	if err != nil {
		return StartUploadResult{}, err
	}

	return StartUploadResult{
		VideoID:          current.ID,
		Title:            current.Title,
		Description:      current.Description,
		Category:         current.Category,
		ProcessingStatus: current.ProcessingStatus,
		PublishStatus:    current.PublishStatus,
		UploadExpiresAt:  current.UploadExpiresAt.UTC(),
		CreatedAt:        current.CreatedAt.UTC(),
		Upload:           uploadInfoFromRepository(uploadTarget),
		Created:          created.Created,
	}, nil
}

func (u *videoUsecase) CompleteUpload(ctx context.Context, actor *entity.User, videoID uint64) (VideoStateResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return VideoStateResult{}, err
	}
	if videoID == 0 {
		return VideoStateResult{}, entity.ErrInvalidInput
	}

	detail, err := u.videos.FindOwnedByID(ctx, actor.ID, videoID)
	if err != nil {
		return VideoStateResult{}, err
	}
	if detail == nil || detail.Video == nil {
		return VideoStateResult{}, entity.ErrVideoNotFound
	}

	video := detail.Video
	switch video.ProcessingStatus {
	case entity.VideoProcessingExpired:
		return VideoStateResult{}, entity.ErrUploadExpired
	case entity.VideoProcessingUploaded,
		entity.VideoProcessingProcessing,
		entity.VideoProcessingReady,
		entity.VideoProcessingFailed:
		return videoStateResult(video), nil
	case entity.VideoProcessingUploading:
	default:
		return VideoStateResult{}, entity.ErrVideoStateConflict
	}

	now := time.Now().UTC()
	if !now.Before(video.UploadExpiresAt.UTC()) {
		return VideoStateResult{}, entity.ErrUploadExpired
	}

	objectInfo, err := u.storage.Stat(ctx, video.OriginalObjectKey)
	if errors.Is(err, entity.ErrObjectNotFound) {
		return VideoStateResult{}, entity.ErrVideoSourceInvalid
	}
	if err != nil {
		return VideoStateResult{}, err
	}
	if objectInfo.SizeBytes < 1 || objectInfo.SizeBytes > maxVideoSizeBytes || !isAllowedSourceContentType(objectInfo.ContentType) {
		return VideoStateResult{}, entity.ErrVideoSourceInvalid
	}

	completed, err := u.videos.CompleteUpload(ctx, actor.ID, video.ID, time.Now().UTC())
	if err != nil {
		return VideoStateResult{}, err
	}

	return videoStateResult(completed), nil
}

func (u *videoUsecase) ListReels(ctx context.Context, viewer *entity.User, input PublicVideoListInput) (PublicVideoListResult, error) {
	viewerID, err := optionalViewerID(viewer)
	if err != nil {
		return PublicVideoListResult{}, err
	}
	if err := validatePublicVideoListInput(input); err != nil {
		return PublicVideoListResult{}, err
	}

	filterHash, err := publicVideoFilterHash(input.Title, input.Category)
	if err != nil {
		return PublicVideoListResult{}, err
	}

	resultType, err := publicSearchResultType(input, filterHash)
	if err != nil {
		return PublicVideoListResult{}, err
	}

	page, err := u.videos.ListPublic(ctx, repositoryPublicVideoListInput(input, viewerID, filterHash, resultType))
	if err != nil {
		return PublicVideoListResult{}, err
	}

	if input.Cursor == nil &&
		resultType == PublicSearchMatched &&
		len(page.Items) == 0 &&
		input.Title != "" {
		resultType = PublicSearchSimilar
		page, err = u.videos.ListPublic(ctx, repositoryPublicVideoListInput(input, viewerID, filterHash, resultType))
		if err != nil {
			return PublicVideoListResult{}, err
		}
	}

	items := make([]PublicVideoResult, 0, len(page.Items))
	for _, item := range page.Items {
		result, err := buildPublicVideoResult(ctx, u.storage, u.readURLTTL, item)
		if err != nil {
			return PublicVideoListResult{}, err
		}
		items = append(items, result)
	}

	output := PublicVideoListResult{
		Items:      items,
		ResultType: resultType,
		HasMore:    page.HasMore,
	}
	if page.HasMore && len(items) > 0 {
		similarity := float32(0)
		if resultType == PublicSearchSimilar {
			similarity = page.LastSimilarity
		}
		cursor, err := encodePublicVideoCursor(PublicVideoCursor{
			ResultType: resultType,
			Similarity: similarity,
			CreatedAt:  page.LastCreatedAt.UTC(),
			ID:         page.LastID,
			FilterHash: filterHash,
		})
		if err != nil {
			return PublicVideoListResult{}, err
		}
		output.NextCursor = &cursor
	}

	return output, nil
}

func (u *videoUsecase) GetDetail(ctx context.Context, viewer *entity.User, videoID uint64) (PublicVideoResult, error) {
	viewerID, err := optionalViewerID(viewer)
	if err != nil {
		return PublicVideoResult{}, err
	}
	if videoID == 0 {
		return PublicVideoResult{}, entity.ErrInvalidInput
	}

	item, err := u.videos.FindPublicByID(ctx, videoID, viewerID)
	if err != nil {
		return PublicVideoResult{}, err
	}
	if item == nil {
		return PublicVideoResult{}, entity.ErrVideoNotFound
	}

	return buildPublicVideoResult(ctx, u.storage, u.readURLTTL, *item)
}

func (u *videoUsecase) ListMine(ctx context.Context, actor *entity.User, input VideoListInput) (OwnedVideoListResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return OwnedVideoListResult{}, err
	}
	if err := validateVideoListInput(input); err != nil {
		return OwnedVideoListResult{}, err
	}

	page, err := u.videos.ListOwned(ctx, actor.ID, input.Limit, repositoryVideoCursor(input.Cursor))
	if err != nil {
		return OwnedVideoListResult{}, err
	}

	items := make([]OwnedVideoResult, 0, len(page.Items))
	for _, item := range page.Items {
		result := OwnedVideoResult{
			ID:               item.ID,
			Category:         item.Category,
			Title:            item.Title,
			Description:      item.Description,
			ProcessingStatus: item.ProcessingStatus,
			PublishStatus:    item.PublishStatus,
			UploadExpiresAt:  item.UploadExpiresAt.UTC(),
			CreatedAt:        item.CreatedAt.UTC(),
			UpdatedAt:        item.UpdatedAt.UTC(),
		}

		if item.ProcessingStatus == entity.VideoProcessingReady &&
			(item.PublishStatus == entity.VideoPublishPrivate ||
				item.PublishStatus == entity.VideoPublishPublished) &&
			strings.TrimSpace(item.ThumbnailObjectKey) != "" {

			thumbnail, err := u.storage.CreateReadURL(
				ctx,
				item.ThumbnailObjectKey,
				u.readURLTTL,
			)
			if err != nil {
				return OwnedVideoListResult{}, err
			}

			readInfo := readInfoFromRepository(thumbnail)
			result.Thumbnail = &readInfo
		}

		items = append(items, result)
	}

	output := OwnedVideoListResult{Items: items, HasMore: page.HasMore}
	if page.HasMore && len(items) > 0 {
		cursor, err := encodeVideoCursor(VideoCursor{CreatedAt: page.LastCreatedAt.UTC(), ID: page.LastID})
		if err != nil {
			return OwnedVideoListResult{}, err
		}
		output.NextCursor = &cursor
	}

	return output, nil
}

func (u *videoUsecase) GetMine(ctx context.Context, actor *entity.User, videoID uint64) (OwnedVideoDetailResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return OwnedVideoDetailResult{}, err
	}
	if videoID == 0 {
		return OwnedVideoDetailResult{}, entity.ErrInvalidInput
	}

	detail, err := u.videos.FindOwnedByID(ctx, actor.ID, videoID)
	if err != nil {
		return OwnedVideoDetailResult{}, err
	}
	if detail == nil || detail.Video == nil {
		return OwnedVideoDetailResult{}, entity.ErrVideoNotFound
	}

	video := detail.Video
	result := OwnedVideoDetailResult{
		ID:               video.ID,
		Category:         video.Category,
		Title:            video.Title,
		Description:      video.Description,
		ProcessingStatus: video.ProcessingStatus,
		PublishStatus:    video.PublishStatus,
		UploadExpiresAt:  video.UploadExpiresAt.UTC(),
		CreatedAt:        video.CreatedAt.UTC(),
		UpdatedAt:        video.UpdatedAt.UTC(),
		FailureCode:      detail.FailureCode,
	}
	if detail.SourceMeta != nil {
		result.SourceMeta = &SourceVideoMetaResult{
			MIMEType:       detail.SourceMeta.MIMEType,
			Container:      detail.SourceMeta.Container,
			SizeBytes:      detail.SourceMeta.SizeBytes,
			DurationMillis: detail.SourceMeta.DurationMillis,
			Width:          detail.SourceMeta.Width,
			Height:         detail.SourceMeta.Height,
			FrameRate:      detail.SourceMeta.FrameRate,
			VideoCodec:     detail.SourceMeta.VideoCodec,
			HasAudio:       detail.SourceMeta.HasAudio,
			AudioCodec:     detail.SourceMeta.AudioCodec,
			CreatedAt:      detail.SourceMeta.CreatedAt.UTC(),
		}
	}

	if video.ProcessingStatus == entity.VideoProcessingReady &&
		(video.PublishStatus == entity.VideoPublishPrivate ||
			video.PublishStatus == entity.VideoPublishPublished) &&
		detail.OutputMeta != nil {

		videoTarget, err := u.storage.CreateReadURL(
			ctx,
			detail.OutputMeta.VideoObjectKey,
			u.readURLTTL,
		)
		if err != nil {
			return OwnedVideoDetailResult{}, err
		}

		thumbnailTarget, err := u.storage.CreateReadURL(
			ctx,
			detail.OutputMeta.ThumbnailObjectKey,
			u.readURLTTL,
		)
		if err != nil {
			return OwnedVideoDetailResult{}, err
		}

		result.OutputMeta = &OwnedOutputMetaResult{
			Container:  detail.OutputMeta.Container,
			Width:      detail.OutputMeta.Width,
			Height:     detail.OutputMeta.Height,
			FrameRate:  detail.OutputMeta.FrameRate,
			VideoCodec: detail.OutputMeta.VideoCodec,
			HasAudio:   detail.OutputMeta.HasAudio,
			AudioCodec: detail.OutputMeta.AudioCodec,
			Video:      readInfoFromRepository(videoTarget),
			Thumbnail:  readInfoFromRepository(thumbnailTarget),
		}
	}

	return result, nil
}

func (u *videoUsecase) SetPrivate(ctx context.Context, actor *entity.User, videoID uint64) (VideoStateResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return VideoStateResult{}, err
	}
	if videoID == 0 {
		return VideoStateResult{}, entity.ErrInvalidInput
	}

	video, err := u.videos.SetPrivateByOwner(ctx, actor.ID, videoID, time.Now().UTC())
	if err != nil {
		return VideoStateResult{}, err
	}
	return videoStateResult(video), nil
}

func (u *videoUsecase) Republish(ctx context.Context, actor *entity.User, videoID uint64) (VideoStateResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return VideoStateResult{}, err
	}
	if videoID == 0 {
		return VideoStateResult{}, entity.ErrInvalidInput
	}

	video, err := u.videos.RepublishByOwner(ctx, actor.ID, videoID, time.Now().UTC())
	if err != nil {
		return VideoStateResult{}, err
	}
	return videoStateResult(video), nil
}

func (u *videoUsecase) Delete(ctx context.Context, actor *entity.User, videoID uint64) error {
	if err := validateActiveActor(actor); err != nil {
		return err
	}
	if videoID == 0 {
		return entity.ErrInvalidInput
	}
	return u.videos.DeleteByOwner(ctx, actor.ID, videoID, time.Now().UTC())
}

func validateStartUploadInput(input StartUploadInput, idempotencyKey string) error {
	if strings.TrimSpace(idempotencyKey) == "" || !input.Category.IsValid() || !isAllowedSourceContentType(input.ContentType) || input.DeclaredSize < 1 || input.DeclaredSize > maxVideoSizeBytes {
		return entity.ErrInvalidInput
	}
	if strings.TrimSpace(input.Title) == "" {
		return entity.ErrInvalidInput
	}
	return nil
}

func validatePublicVideoListInput(input PublicVideoListInput) error {
	if input.Limit < 1 || input.Limit > 100 {
		return entity.ErrInvalidInput
	}
	if input.Category != nil && !input.Category.IsValid() {
		return entity.ErrInvalidInput
	}
	if input.Cursor != nil {
		if !input.Cursor.ResultType.IsValid() ||
			input.Cursor.CreatedAt.IsZero() ||
			input.Cursor.ID == 0 ||
			len(input.Cursor.FilterHash) != sha256.Size*2 {
			return entity.ErrCursorInvalid
		}
	}
	return nil
}

func publicSearchResultType(input PublicVideoListInput, filterHash string) (PublicSearchResultType, error) {
	if input.Cursor != nil {
		if input.Cursor.FilterHash != filterHash {
			return "", entity.ErrCursorInvalid
		}
		switch input.Cursor.ResultType {
		case PublicSearchAll:
			if input.Title != "" || input.Category != nil {
				return "", entity.ErrCursorInvalid
			}
		case PublicSearchMatched:
			if input.Title == "" && input.Category == nil {
				return "", entity.ErrCursorInvalid
			}
		case PublicSearchSimilar:
			if input.Title == "" {
				return "", entity.ErrCursorInvalid
			}
		default:
			return "", entity.ErrCursorInvalid
		}
		return input.Cursor.ResultType, nil
	}

	if input.Title == "" && input.Category == nil {
		return PublicSearchAll, nil
	}
	return PublicSearchMatched, nil
}

func publicVideoFilterHash(title string, category *entity.CategoryCode) (string, error) {
	type filterPayload struct {
		Title    string `json:"title"`
		Category string `json:"category"`
	}

	categoryValue := ""
	if category != nil {
		categoryValue = string(*category)
	}
	payload, err := json.Marshal(filterPayload{
		Title:    title,
		Category: categoryValue,
	})
	if err != nil {
		return "", fmt.Errorf("encode public video filter: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func repositoryPublicVideoListInput(
	input PublicVideoListInput,
	viewerID *uint64,
	filterHash string,
	resultType PublicSearchResultType,
) repository.PublicVideoListInput {
	var cursor *repository.PublicVideoCursor
	if input.Cursor != nil {
		cursor = &repository.PublicVideoCursor{
			ResultType: repository.PublicVideoSearchMode(input.Cursor.ResultType),
			Similarity: input.Cursor.Similarity,
			CreatedAt:  input.Cursor.CreatedAt.UTC(),
			ID:         input.Cursor.ID,
			FilterHash: filterHash,
		}
	}

	return repository.PublicVideoListInput{
		Title:        input.Title,
		Category:     input.Category,
		Limit:        input.Limit,
		Cursor:       cursor,
		ViewerUserID: viewerID,
		SearchMode:   repository.PublicVideoSearchMode(resultType),
	}
}

func validateVideoListInput(input VideoListInput) error {
	if input.Limit < 1 || input.Limit > 100 {
		return entity.ErrInvalidInput
	}
	if input.Cursor != nil && (input.Cursor.CreatedAt.IsZero() || input.Cursor.ID == 0) {
		return entity.ErrCursorInvalid
	}
	return nil
}

func validateActiveActor(actor *entity.User) error {
	if actor == nil || actor.ID == 0 {
		return entity.ErrUnauthorized
	}
	if !actor.IsActive() {
		return entity.ErrUserSuspended
	}
	return nil
}

func optionalViewerID(viewer *entity.User) (*uint64, error) {
	if viewer == nil {
		return nil, nil
	}
	if err := validateActiveActor(viewer); err != nil {
		return nil, err
	}
	viewerID := viewer.ID
	return &viewerID, nil
}

func videoStateResult(video *entity.Video) VideoStateResult {
	if video == nil {
		return VideoStateResult{}
	}
	return VideoStateResult{
		ID:               video.ID,
		ProcessingStatus: video.ProcessingStatus,
		PublishStatus:    video.PublishStatus,
		UpdatedAt:        video.UpdatedAt.UTC(),
	}
}

func repositoryVideoCursor(cursor *VideoCursor) *repository.VideoCursor {
	if cursor == nil {
		return nil
	}
	return &repository.VideoCursor{CreatedAt: cursor.CreatedAt.UTC(), ID: cursor.ID}
}

func encodeVideoCursor(cursor VideoCursor) (string, error) {
	payload, err := json.Marshal(VideoCursor{CreatedAt: cursor.CreatedAt.UTC(), ID: cursor.ID})
	if err != nil {
		return "", fmt.Errorf("encode video cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func encodePublicVideoCursor(cursor PublicVideoCursor) (string, error) {
	payload, err := json.Marshal(PublicVideoCursor{
		ResultType: cursor.ResultType,
		Similarity: cursor.Similarity,
		CreatedAt:  cursor.CreatedAt.UTC(),
		ID:         cursor.ID,
		FilterHash: cursor.FilterHash,
	})
	if err != nil {
		return "", fmt.Errorf("encode public video cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func buildPublicVideoResult(ctx context.Context, storage repository.IObjectStorageRepository, readURLTTL time.Duration, item repository.PublicVideoItem) (PublicVideoResult, error) {
	videoTarget, err := storage.CreateReadURL(ctx, item.VideoObjectKey, readURLTTL)
	if err != nil {
		return PublicVideoResult{}, err
	}
	thumbnailTarget, err := storage.CreateReadURL(ctx, item.ThumbnailObjectKey, readURLTTL)
	if err != nil {
		return PublicVideoResult{}, err
	}

	return PublicVideoResult{
		ID:          item.ID,
		UserID:      item.UserID,
		AuthorName:  item.AuthorName,
		Category:    item.Category,
		Title:       item.Title,
		Description: item.Description,
		CreatedAt:   item.CreatedAt.UTC(),
		Video:       readInfoFromRepository(videoTarget),
		Thumbnail:   readInfoFromRepository(thumbnailTarget),
		LikeCount:   item.LikeCount,
		IsLiked:     item.IsLiked,
		IsSaved:     item.IsSaved,
	}, nil
}

func uploadInfoFromRepository(target repository.UploadTarget) UploadInfo {
	return UploadInfo{
		Method:      target.Method,
		URL:         target.URL,
		ContentType: target.ContentType,
		ExpiresAt:   target.ExpiresAt.UTC(),
	}
}

func readInfoFromRepository(target repository.ReadTarget) ReadInfo {
	return ReadInfo{URL: target.URL, ExpiresAt: target.ExpiresAt.UTC()}
}

func hashIdempotencyKey(key []byte, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func hashStartUploadRequest(input StartUploadInput) (string, error) {
	type requestHashPayload struct {
		Title        string              `json:"title"`
		Description  string              `json:"description"`
		Category     entity.CategoryCode `json:"category"`
		ContentType  string              `json:"content_type"`
		DeclaredSize int64               `json:"declared_size"`
	}

	payload, err := json.Marshal(requestHashPayload{
		Title:        input.Title,
		Description:  input.Description,
		Category:     input.Category,
		ContentType:  input.ContentType,
		DeclaredSize: input.DeclaredSize,
	})
	if err != nil {
		return "", fmt.Errorf("encode upload request hash: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func buildSourceObjectKey(prefix, extension string) (string, error) {
	if prefix != "videos/" || strings.TrimSpace(extension) == "" {
		return "", entity.ErrInvalidInput
	}
	randomID, err := generateRandomHex(videoObjectRandomBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%ssource/%s.%s", prefix, randomID, extension), nil
}

func generateRandomHex(size int) (string, error) {
	if size < 1 {
		return "", entity.ErrInvalidInput
	}
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate object key random id: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func sourceExtension(contentType string) string {
	if normalizeContentType(contentType) == "video/quicktime" {
		return "mov"
	}
	return "mp4"
}

func isAllowedSourceContentType(contentType string) bool {
	contentType = normalizeContentType(contentType)
	return contentType == "video/mp4" || contentType == "video/quicktime"
}

func normalizeContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if index := strings.IndexByte(contentType, ';'); index >= 0 {
		contentType = strings.TrimSpace(contentType[:index])
	}
	return contentType
}

func normalizeUsecaseManagedPrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" || strings.Contains(value, "\\") {
		return ""
	}
	return value + "/"
}
