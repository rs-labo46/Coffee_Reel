package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"coffee-reel/entity"
	"coffee-reel/usecase"
	"coffee-reel/validator"

	"github.com/labstack/echo/v4"
)

type IAdminVideoController interface {
	List(c echo.Context) error
	Detail(c echo.Context) error
	Hide(c echo.Context) error
	Restore(c echo.Context) error
}

type adminVideoController struct {
	videos    usecase.IAdminVideoUsecase
	validator validator.IAdminVideoValidator
}

type adminVideoAuthorResponse struct {
	ID     uint64            `json:"id"`
	Name   string            `json:"name"`
	Status entity.UserStatus `json:"status"`
}

type adminVideoListItemResponse struct {
	ID               uint64                       `json:"id"`
	Author           adminVideoAuthorResponse     `json:"author"`
	Title            string                       `json:"title"`
	Description      string                       `json:"description"`
	Category         entity.CategoryCode          `json:"category"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

type adminVideoListResponse struct {
	Items      []adminVideoListItemResponse `json:"items"`
	NextCursor *string                      `json:"next_cursor"`
	HasMore    bool                         `json:"has_more"`
}

type adminVideoDetailResponse struct {
	ID               uint64                       `json:"id"`
	Author           adminVideoAuthorResponse     `json:"author"`
	Title            string                       `json:"title"`
	Description      string                       `json:"description"`
	Category         entity.CategoryCode          `json:"category"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
	PlaybackURL      *string                      `json:"playback_url"`
	ThumbnailURL     *string                      `json:"thumbnail_url"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

type adminVideoStateResponse struct {
	ID               uint64                       `json:"id"`
	ProcessingStatus entity.VideoProcessingStatus `json:"processing_status"`
	PublishStatus    entity.VideoPublishStatus    `json:"publish_status"`
	UpdatedAt        time.Time                    `json:"updated_at"`
}

func NewAdminVideoController(videos usecase.IAdminVideoUsecase, adminValidator validator.IAdminVideoValidator) IAdminVideoController {
	return &adminVideoController{
		videos:    videos,
		validator: adminValidator,
	}
}

func (a *adminVideoController) List(c echo.Context) error {
	actor, err := adminUserFromContext(c)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	input, err := a.validator.ValidateListQuery(
		c.QueryParam("limit"),
		c.QueryParam("cursor"),
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	result, err := a.videos.List(
		c.Request().Context(),
		actor,
		input,
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	items := make(
		[]adminVideoListItemResponse,
		0,
		len(result.Items),
	)

	for _, item := range result.Items {
		items = append(items, adminVideoListItemResponse{
			ID: item.ID,
			Author: adminVideoAuthorResponse{
				ID:     item.Author.ID,
				Name:   item.Author.Name,
				Status: item.Author.Status,
			},
			Title:            item.Title,
			Description:      item.Description,
			Category:         item.Category,
			ProcessingStatus: item.ProcessingStatus,
			PublishStatus:    item.PublishStatus,
			CreatedAt:        item.CreatedAt.UTC(),
			UpdatedAt:        item.UpdatedAt.UTC(),
		})
	}

	return c.JSON(http.StatusOK, adminVideoListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (a *adminVideoController) Detail(c echo.Context) error {
	actor, err := adminUserFromContext(c)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	videoID, err := a.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	result, err := a.videos.Get(
		c.Request().Context(),
		actor,
		videoID,
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	return c.JSON(http.StatusOK, adminVideoDetailResponse{
		ID: result.ID,
		Author: adminVideoAuthorResponse{
			ID:     result.Author.ID,
			Name:   result.Author.Name,
			Status: result.Author.Status,
		},
		Title:            result.Title,
		Description:      result.Description,
		Category:         result.Category,
		ProcessingStatus: result.ProcessingStatus,
		PublishStatus:    result.PublishStatus,
		PlaybackURL:      result.PlaybackURL,
		ThumbnailURL:     result.ThumbnailURL,
		CreatedAt:        result.CreatedAt.UTC(),
		UpdatedAt:        result.UpdatedAt.UTC(),
	})
}

func (a *adminVideoController) Hide(c echo.Context) error {
	return a.changeState(c, a.videos.Hide)
}

func (a *adminVideoController) Restore(c echo.Context) error {
	return a.changeState(c, a.videos.Restore)
}

func (a *adminVideoController) changeState(c echo.Context,
	change func(context.Context, *entity.User, uint64, string, string) (usecase.AdminVideoStateResult, error),
) error {
	actor, err := adminUserFromContext(c)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	id := strings.TrimSpace(requestID(c))
	if id == "" {
		return writeAPIError(
			c,
			http.StatusInternalServerError,
			"internal_error",
			"内部エラーが発生しました",
		)
	}

	videoID, err := a.validator.ValidateVideoID(
		c.Param("video_id"),
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	reason, err := a.validator.ValidateReasonRequest(
		c.Request().Body,
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	result, err := change(
		c.Request().Context(),
		actor,
		videoID,
		reason,
		id,
	)
	if err != nil {
		return writeAdminVideoError(c, err)
	}

	return c.JSON(http.StatusOK, adminVideoStateResponse{
		ID:               result.ID,
		ProcessingStatus: result.ProcessingStatus,
		PublishStatus:    result.PublishStatus,
		UpdatedAt:        result.UpdatedAt.UTC(),
	})
}

func writeAdminVideoError(
	c echo.Context,
	err error,
) error {
	switch {
	case errors.Is(err, entity.ErrInvalidInput),
		errors.Is(err, entity.ErrCursorInvalid):

		return writeAPIError(
			c,
			http.StatusBadRequest,
			"validation_failed",
			"入力内容が正しくありません",
		)

	case errors.Is(err, entity.ErrUnauthorized):
		return writeAPIError(
			c,
			http.StatusUnauthorized,
			"unauthorized",
			"認証情報が無効です",
		)

	case errors.Is(err, entity.ErrAdminRequired):
		return writeAPIError(
			c,
			http.StatusForbidden,
			"admin_required",
			"管理者権限が必要です",
		)

	case errors.Is(err, entity.ErrVideoNotFound):
		return writeAPIError(
			c,
			http.StatusNotFound,
			"video_not_found",
			"対象投稿が見つかりません",
		)

	case errors.Is(err, entity.ErrUserSuspended):
		return writeAPIError(
			c,
			http.StatusConflict,
			"video_owner_suspended",
			"投稿者が利用停止中のため公開を再開できません",
		)

	case errors.Is(err, entity.ErrVideoStateConflict):
		return writeAPIError(
			c,
			http.StatusConflict,
			"video_state_conflict",
			"投稿の現在状態ではこの操作を実行できません",
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
