package controller

import (
	"context"
	"net/http"

	"coffee-reel/entity"

	"coffee-reel/usecase"
	"coffee-reel/validator"

	"github.com/labstack/echo/v4"
)

type IVideoLikeController interface {
	Like(c echo.Context) error
	Unlike(c echo.Context) error
}

type videoLikeController struct {
	likes     usecase.IVideoLikeUsecase
	validator validator.IVideoValidator
}

type videoLikeResponse struct {
	VideoID   uint64 `json:"video_id"`
	LikeCount int64  `json:"like_count"`
	IsLiked   bool   `json:"is_liked"`
}

func NewVideoLikeController(likes usecase.IVideoLikeUsecase, videoValidator validator.IVideoValidator) IVideoLikeController {
	return &videoLikeController{
		likes:     likes,
		validator: videoValidator,
	}
}

func (v *videoLikeController) Like(c echo.Context) error {
	return v.change(c, v.likes.Like)
}

func (v *videoLikeController) Unlike(c echo.Context) error {
	return v.change(c, v.likes.Unlike)
}

func (v *videoLikeController) change(
	c echo.Context,
	change func(ctx context.Context, actor *entity.User, videoID uint64) (usecase.VideoLikeResult, error),
) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := v.validator.ValidateVideoID(c.Param("video_id"))
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := change(c.Request().Context(), actor, videoID)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(http.StatusOK, videoLikeResponse{
		VideoID:   result.VideoID,
		LikeCount: result.LikeCount,
		IsLiked:   result.IsLiked,
	})
}
