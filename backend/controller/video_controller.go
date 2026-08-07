package controller

import (
	"context"
	"errors"
	"net/http"
	"time"

	"coffee-reel/entity"
	"coffee-reel/usecase"
	"coffee-reel/validator"

	"github.com/labstack/echo/v4"
)

const idempotencyKeyHeader = "Idempotency-Key"

type IVideoController interface {
	StartUpload(c echo.Context) error
	CompleteUpload(c echo.Context) error
	ListReels(c echo.Context) error
	Detail(c echo.Context) error
	ListMine(c echo.Context) error
	MineDetail(c echo.Context) error
	SetPrivate(c echo.Context) error
	Republish(c echo.Context) error
	Delete(c echo.Context) error
}

type videoController struct {
	videos    usecase.IVideoUsecase
	validator validator.IVideoValidator
}

type startUploadRequest struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	FileContentType string `json:"file_content_type"`
	FileSizeBytes   int64  `json:"file_size_bytes"`
}

type startUploadResponse struct {
	Video  startUploadVideoResponse `json:"video"`
	Upload uploadResponse           `json:"upload"`
}

type startUploadVideoResponse struct {
	ID               uint64                       `json:"id"`
	Title            string                       `json:"title"`
	Description      string                       `json:"description"`
	Category         entity.CategoryCode          `json:"category"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
	UploadExpiresAt  time.Time                    `json:"upload_expires_at"`
	CreatedAt        time.Time                    `json:"created_at"`
}

type uploadResponse struct {
	Method    string                `json:"method"`
	URL       string                `json:"url"`
	Headers   uploadHeadersResponse `json:"headers"`
	ExpiresAt time.Time             `json:"expires_at"`
}

type uploadHeadersResponse struct {
	ContentType string `json:"Content-Type"`
}

type videoStateResponse struct {
	ID               uint64                       `json:"id"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
}

type videoAuthorResponse struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type publicVideoResponse struct {
	ID           uint64              `json:"id"`
	Title        string              `json:"title"`
	Description  string              `json:"description"`
	Category     entity.CategoryCode `json:"category"`
	Author       videoAuthorResponse `json:"author"`
	PlaybackURL  string              `json:"playback_url"`
	ThumbnailURL string              `json:"thumbnail_url"`
	IsSaved      bool                `json:"is_saved"`
	CreatedAt    time.Time           `json:"created_at"`
}

type publicVideoListResponse struct {
	Items      []publicVideoResponse          `json:"items"`
	ResultType usecase.PublicSearchResultType `json:"result_type,omitempty"`
	NextCursor *string                        `json:"next_cursor"`
	HasMore    bool                           `json:"has_more"`
}

type ownedVideoResponse struct {
	ID               uint64                       `json:"id"`
	Title            string                       `json:"title"`
	Category         entity.CategoryCode          `json:"category"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
	ThumbnailURL     *string                      `json:"thumbnail_url"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

type ownedVideoListResponse struct {
	Items      []ownedVideoResponse `json:"items"`
	NextCursor *string              `json:"next_cursor"`
	HasMore    bool                 `json:"has_more"`
}
type publicVideoListRequest struct {
	Title    string `query:"title"`
	Category string `query:"category"`
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
}

type ownedVideoDetailResponse struct {
	ID               uint64                       `json:"id"`
	Title            string                       `json:"title"`
	Description      string                       `json:"description"`
	Category         entity.CategoryCode          `json:"category"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
	FailureCode      *entity.VideoFailureCode     `json:"failure_code"`
	PlaybackURL      *string                      `json:"playback_url"`
	ThumbnailURL     *string                      `json:"thumbnail_url"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

func NewVideoController(videos usecase.IVideoUsecase, videoValidator validator.IVideoValidator) IVideoController {
	return &videoController{
		videos:    videos,
		validator: videoValidator,
	}
}

func (v *videoController) StartUpload(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	var req startUploadRequest
	if err := c.Bind(&req); err != nil {
		return writeVideoError(c, entity.ErrInvalidInput)
	}

	input, err := v.validator.ValidateStartUpload(
		req.Title,
		req.Description,
		req.Category,
		req.FileContentType,
		req.FileSizeBytes,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	idempotencyKey, err := v.validator.ValidateIdempotencyKey(
		c.Request().Header.Get(idempotencyKeyHeader),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := v.videos.StartUpload(
		c.Request().Context(),
		actor,
		input,
		idempotencyKey,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(http.StatusCreated, startUploadResponse{
		Video: startUploadVideoResponse{
			ID:               result.VideoID,
			Title:            result.Title,
			Description:      result.Description,
			Category:         result.Category,
			ProcessingStatus: result.ProcessingStatus,
			PublishStatus:    result.PublishStatus,
			UploadExpiresAt:  result.UploadExpiresAt,
			CreatedAt:        result.CreatedAt,
		},
		Upload: uploadResponse{
			Method: result.Upload.Method,
			URL:    result.Upload.URL,
			Headers: uploadHeadersResponse{
				ContentType: result.Upload.ContentType,
			},
			ExpiresAt: result.Upload.ExpiresAt,
		},
	})
}

func (v *videoController) CompleteUpload(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := v.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := v.videos.CompleteUpload(
		c.Request().Context(),
		actor,
		videoID,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(
		http.StatusAccepted,
		newVideoStateResponse(result),
	)
}

func (v *videoController) ListReels(c echo.Context) error {
	viewer, err := optionalVideoViewerFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	var req publicVideoListRequest
	if err := c.Bind(&req); err != nil {
		return writeVideoError(c, entity.ErrInvalidInput)
	}
	query := c.QueryParams()
	_, titleSpecified := query["title"]
	_, categorySpecified := query["category"]

	input, err := v.validator.ValidatePublicListQuery(
		req.Title,
		titleSpecified,
		req.Category,
		categorySpecified,
		req.Limit,
		req.Cursor,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := v.videos.ListReels(
		c.Request().Context(),
		viewer,
		input,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		newPublicVideoListResponse(result),
	)
}

func (v *videoController) Detail(c echo.Context) error {
	viewer, err := optionalVideoViewerFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := v.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := v.videos.GetDetail(
		c.Request().Context(),
		viewer,
		videoID,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		newPublicVideoResponse(result),
	)
}

func (v *videoController) ListMine(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	input, err := v.validator.ValidateListQuery(
		c.QueryParam("limit"),
		c.QueryParam("cursor"),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := v.videos.ListMine(
		c.Request().Context(),
		actor,
		input,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	items := make(
		[]ownedVideoResponse,
		0,
		len(result.Items),
	)

	for _, item := range result.Items {
		var thumbnailURL *string

		if item.Thumbnail != nil {
			url := item.Thumbnail.URL
			thumbnailURL = &url
		}

		items = append(items, ownedVideoResponse{
			ID:               item.ID,
			Title:            item.Title,
			Category:         item.Category,
			ProcessingStatus: item.ProcessingStatus,
			PublishStatus:    item.PublishStatus,
			ThumbnailURL:     thumbnailURL,
			CreatedAt:        item.CreatedAt,
			UpdatedAt:        item.UpdatedAt,
		})
	}

	return c.JSON(http.StatusOK, ownedVideoListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (v *videoController) MineDetail(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := v.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := v.videos.GetMine(
		c.Request().Context(),
		actor,
		videoID,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	var playbackURL *string
	var thumbnailURL *string

	if result.OutputMeta != nil {
		videoURL := result.OutputMeta.Video.URL
		thumbnail := result.OutputMeta.Thumbnail.URL

		playbackURL = &videoURL
		thumbnailURL = &thumbnail
	}

	return c.JSON(http.StatusOK, ownedVideoDetailResponse{
		ID:               result.ID,
		Title:            result.Title,
		Description:      result.Description,
		Category:         result.Category,
		ProcessingStatus: result.ProcessingStatus,
		PublishStatus:    result.PublishStatus,
		FailureCode:      result.FailureCode,
		PlaybackURL:      playbackURL,
		ThumbnailURL:     thumbnailURL,
		CreatedAt:        result.CreatedAt,
		UpdatedAt:        result.UpdatedAt,
	})
}

func (v *videoController) SetPrivate(c echo.Context) error {
	return v.changePublishStatus(
		c,
		v.videos.SetPrivate,
	)
}

func (v *videoController) Republish(c echo.Context) error {
	return v.changePublishStatus(
		c,
		v.videos.Republish,
	)
}

func (v *videoController) Delete(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := v.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	if err := v.videos.Delete(
		c.Request().Context(),
		actor,
		videoID,
	); err != nil {
		return writeVideoError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (v *videoController) changePublishStatus(c echo.Context, change func(ctx context.Context, actor *entity.User, videoID uint64) (usecase.VideoStateResult, error)) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := v.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := change(
		c.Request().Context(),
		actor,
		videoID,
	)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		newVideoStateResponse(result),
	)
}

func newVideoStateResponse(result usecase.VideoStateResult) videoStateResponse {
	return videoStateResponse{
		ID:               result.ID,
		ProcessingStatus: result.ProcessingStatus,
		PublishStatus:    result.PublishStatus,
	}
}

func newPublicVideoListResponse(result usecase.PublicVideoListResult) publicVideoListResponse {
	items := make(
		[]publicVideoResponse,
		0,
		len(result.Items),
	)

	for _, item := range result.Items {
		items = append(
			items,
			newPublicVideoResponse(item),
		)
	}

	return publicVideoListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

func newPublicVideoResponse(result usecase.PublicVideoResult) publicVideoResponse {
	return publicVideoResponse{
		ID:          result.ID,
		Title:       result.Title,
		Description: result.Description,
		Category:    result.Category,
		Author: videoAuthorResponse{
			ID:   result.UserID,
			Name: result.AuthorName,
		},
		PlaybackURL:  result.Video.URL,
		ThumbnailURL: result.Thumbnail.URL,
		IsSaved:      result.IsSaved,
		CreatedAt:    result.CreatedAt,
	}
}

func videoUserFromContext(c echo.Context) (*entity.User, error) {
	user, ok := c.Get(userContextKey).(*entity.User)
	if !ok || user == nil {
		return nil, entity.ErrUnauthorized
	}

	return user, nil
}

func optionalVideoViewerFromContext(c echo.Context) (*entity.User, error) {
	value := c.Get(userContextKey)
	if value == nil {
		return nil, nil
	}

	user, ok := value.(*entity.User)
	if !ok || user == nil {
		return nil, entity.ErrUnauthorized
	}

	return user, nil
}

func writeVideoError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, entity.ErrInvalidInput),
		errors.Is(err, entity.ErrCursorInvalid):

		return writeAPIError(
			c,
			http.StatusBadRequest,
			"validation_failed",
			"入力内容が正しくありません",
		)

	case errors.Is(err, entity.ErrUnauthorized),
		errors.Is(err, entity.ErrUserSuspended):

		return writeAPIError(
			c,
			http.StatusUnauthorized,
			"unauthorized",
			"認証情報が無効です",
		)

	case errors.Is(err, entity.ErrVideoForbidden):
		return writeAPIError(
			c,
			http.StatusForbidden,
			"video_forbidden",
			"この動画を操作する権限がありません",
		)

	case errors.Is(err, entity.ErrVideoNotFound),
		errors.Is(err, entity.ErrVideoNotPublic):

		return writeAPIError(
			c,
			http.StatusNotFound,
			"video_not_found",
			"対象動画が見つかりません",
		)

	case errors.Is(err, entity.ErrVideoStateConflict):
		return writeAPIError(
			c,
			http.StatusConflict,
			"video_state_conflict",
			"動画の現在状態ではこの操作を実行できません",
		)

	case errors.Is(err, entity.ErrIdempotencyConflict):
		return writeAPIError(
			c,
			http.StatusConflict,
			"idempotency_conflict",
			"同じIdempotency-Keyが異なる入力で使用されています",
		)

	case errors.Is(err, entity.ErrUploadExpired):
		return writeAPIError(
			c,
			http.StatusConflict,
			"upload_expired",
			"アップロード期限が切れています",
		)

	case errors.Is(err, entity.ErrVideoSourceInvalid),
		errors.Is(err, entity.ErrObjectNotFound):

		return writeAPIError(
			c,
			http.StatusConflict,
			"video_source_invalid",
			"アップロード済み動画を確認できません",
		)

	case errors.Is(err, entity.ErrRateLimitExceeded):
		return writeAPIError(
			c,
			http.StatusTooManyRequests,
			"rate_limit_exceeded",
			"リクエスト回数が上限を超えました",
		)

	default:
		return writeAPIError(
			c,
			http.StatusInternalServerError,
			"internal_error",
			"内部エラーが発生しました",
		)
	}
}
