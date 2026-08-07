package controller

import (
	"coffee-reel/entity"
	"coffee-reel/usecase"
	"coffee-reel/validator"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type IAdminUserController interface {
	ListUsers(c echo.Context) error
	GetUserDetail(c echo.Context) error
	SuspendUser(c echo.Context) error
	ResumeUser(c echo.Context) error
}

type adminUserController struct {
	users     usecase.IAdminUserUsecase
	validator validator.IAdminUserValidator
}

type adminUserReasonRequest struct {
	Reason string `json:"reason"`
}

type adminUserListItemResponse struct {
	ID        uint64            `json:"id"`
	Name      string            `json:"name"`
	Email     string            `json:"email"`
	Status    entity.UserStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
}

type adminUserListResponse struct {
	Items      []adminUserListItemResponse `json:"items"`
	NextCursor *string                     `json:"next_cursor"`
	HasMore    bool                        `json:"has_more"`
}

type adminUserVideoResponse struct {
	ID               uint64    `json:"id"`
	Title            string    `json:"title"`
	ProcessingStatus string    `json:"processing_status"`
	PublishStatus    string    `json:"publish_status"`
	CreatedAt        time.Time `json:"created_at"`
}

type adminUserDetailResponse struct {
	ID        uint64                   `json:"id"`
	Name      string                   `json:"name"`
	Email     string                   `json:"email"`
	Status    entity.UserStatus        `json:"status"`
	CreatedAt time.Time                `json:"created_at"`
	Videos    []adminUserVideoResponse `json:"videos"`
}

type adminUserStatusResponse struct {
	ID        uint64            `json:"id"`
	Status    entity.UserStatus `json:"status"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func NewAdminUserController(users usecase.IAdminUserUsecase, adminValidator validator.IAdminUserValidator) IAdminUserController {
	return &adminUserController{users: users, validator: adminValidator}
}

func (a *adminUserController) ListUsers(c echo.Context) error {
	actor, err := adminUserFromContext(c)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	input, err := a.validator.ValidateUserListQuery(c.QueryParam("limit"), c.QueryParam("cursor"))
	if err != nil {
		return writeAdminUserError(c, err)
	}

	result, err := a.users.ListUsers(c.Request().Context(), actor, input)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	items := make([]adminUserListItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, adminUserListItemResponse{
			ID:        item.ID,
			Name:      item.Name,
			Email:     item.Email,
			Status:    item.Status,
			CreatedAt: item.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, adminUserListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (a *adminUserController) GetUserDetail(c echo.Context) error {
	actor, err := adminUserFromContext(c)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	targetUserID, err := a.validator.ValidateUserID(c.Param("user_id"))
	if err != nil {
		return writeAdminUserError(c, err)
	}

	result, err := a.users.GetUserDetail(c.Request().Context(), actor, targetUserID)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	videos := make([]adminUserVideoResponse, 0, len(result.Videos))
	for _, video := range result.Videos {
		videos = append(videos, adminUserVideoResponse{
			ID:               video.ID,
			Title:            video.Title,
			ProcessingStatus: video.ProcessingStatus,
			PublishStatus:    video.PublishStatus,
			CreatedAt:        video.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, adminUserDetailResponse{
		ID:        result.ID,
		Name:      result.Name,
		Email:     result.Email,
		Status:    result.Status,
		CreatedAt: result.CreatedAt,
		Videos:    videos,
	})
}

func (a *adminUserController) SuspendUser(c echo.Context) error {
	return a.changeUserStatus(c, a.users.SuspendUser)
}

func (a *adminUserController) ResumeUser(c echo.Context) error {
	return a.changeUserStatus(c, a.users.ResumeUser)
}

func (a *adminUserController) changeUserStatus(c echo.Context, change func(ctx context.Context, actor *entity.User, targetUserID uint64, reason string, requestID string) (usecase.AdminUserStatusResult, error)) error {
	actor, err := adminUserFromContext(c)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	targetUserID, err := a.validator.ValidateUserID(c.Param("user_id"))
	if err != nil {
		return writeAdminUserError(c, err)
	}

	var req adminUserReasonRequest
	if err := c.Bind(&req); err != nil {
		return writeAdminUserError(c, entity.ErrInvalidInput)
	}

	reason, err := a.validator.ValidateReason(req.Reason)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	result, err := change(
		c.Request().Context(),
		actor,
		targetUserID,
		reason,
		requestID(c),
	)
	if err != nil {
		return writeAdminUserError(c, err)
	}

	return c.JSON(http.StatusOK, adminUserStatusResponse{
		ID:        result.ID,
		Status:    result.Status,
		UpdatedAt: result.UpdatedAt,
	})
}

func adminUserFromContext(c echo.Context) (*entity.User, error) {
	user, ok := c.Get(userContextKey).(*entity.User)
	if !ok || user == nil {
		return nil, entity.ErrUnauthorized
	}
	return user, nil
}

func writeAdminUserError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, entity.ErrInvalidInput):
		return writeAPIError(c, http.StatusBadRequest, "validation_failed", "入力内容が正しくありません")
	case errors.Is(err, entity.ErrUnauthorized):
		return writeAPIError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")
	case errors.Is(err, entity.ErrAdminRequired):
		return writeAPIError(c, http.StatusForbidden, "admin_required", "管理者権限が必要です")
	case errors.Is(err, entity.ErrUserManagementForbidden):
		return writeAPIError(c, http.StatusForbidden, "user_management_forbidden", "このユーザーは管理対象外です")
	case errors.Is(err, entity.ErrUserNotFound):
		return writeAPIError(c, http.StatusNotFound, "user_not_found", "対象ユーザーが見つかりません")
	case errors.Is(err, entity.ErrUserStatusConflict):
		return writeAPIError(c, http.StatusConflict, "user_status_conflict", "ユーザーの状態が既に変更されています")
	default:
		return writeAPIError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
	}
}
