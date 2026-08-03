package controller

import (
	"net/http"

	"coffee-reel/usecase"
	"coffee-reel/validator"

	"github.com/labstack/echo/v4"
)

type ISavedVideoController interface {
	Save(c echo.Context) error
	Remove(c echo.Context) error
	List(c echo.Context) error
}

type savedVideoController struct {
	saved     usecase.ISavedVideoUsecase
	validator validator.IVideoValidator
}

func NewSavedVideoController(saved usecase.ISavedVideoUsecase, videoValidator validator.IVideoValidator) ISavedVideoController {
	return &savedVideoController{
		saved:     saved,
		validator: videoValidator,
	}
}

func (s *savedVideoController) Save(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := s.validator.ValidateVideoID(c.Param("video_id"))
	if err != nil {
		return writeVideoError(c, err)
	}

	if err := s.saved.Save(c.Request().Context(), actor, videoID); err != nil {
		return writeVideoError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (s *savedVideoController) Remove(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	videoID, err := s.validator.ValidateVideoID(c.Param("video_id"))
	if err != nil {
		return writeVideoError(c, err)
	}

	if err := s.saved.Remove(c.Request().Context(), actor, videoID); err != nil {
		return writeVideoError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (s *savedVideoController) List(c echo.Context) error {
	actor, err := videoUserFromContext(c)
	if err != nil {
		return writeVideoError(c, err)
	}

	input, err := s.validator.ValidateListQuery(c.QueryParam("limit"), c.QueryParam("cursor"))
	if err != nil {
		return writeVideoError(c, err)
	}

	result, err := s.saved.List(c.Request().Context(), actor, input)
	if err != nil {
		return writeVideoError(c, err)
	}

	return c.JSON(http.StatusOK, newPublicVideoListResponse(result))
}
