package usecase

import (
	"coffee-reel/entity"
	"coffee-reel/repository"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type IAdminUserUsecase interface {
	CreateAdmin(ctx context.Context, name, email, password string) (*entity.User, bool, error)
	ListUsers(ctx context.Context, actor *entity.User, input AdminUserListInput) (AdminUserListResult, error)
	GetUserDetail(ctx context.Context, actor *entity.User, targetUserID uint64) (AdminUserDetailResult, error)
	SuspendUser(ctx context.Context, actor *entity.User, targetUserID uint64, reason, requestID string) (AdminUserStatusResult, error)
	ResumeUser(ctx context.Context, actor *entity.User, targetUserID uint64, reason, requestID string) (AdminUserStatusResult, error)
}

type AdminUserCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uint64    `json:"id"`
}

type AdminUserListInput struct {
	Limit  int
	Cursor *AdminUserCursor
}

type AdminUserListItem struct {
	ID        uint64
	Name      string
	Email     string
	Status    entity.UserStatus
	CreatedAt time.Time
}

type AdminUserListResult struct {
	Items      []AdminUserListItem
	NextCursor *string
	HasMore    bool
}

type AdminUserVideoItem struct {
	ID               uint64
	Title            string
	ProcessingStatus string
	PublishStatus    string
	CreatedAt        time.Time
}

type AdminUserDetailResult struct {
	ID        uint64
	Name      string
	Email     string
	Status    entity.UserStatus
	CreatedAt time.Time
	Videos    []AdminUserVideoItem
}

type AdminUserStatusResult struct {
	ID        uint64
	Status    entity.UserStatus
	UpdatedAt time.Time
}

type adminUserUsecase struct {
	users      repository.IUserRepository
	adminUsers repository.IAdminUserRepository
	tokens     ITokenService
}

func NewAdminUserUsecase(users repository.IUserRepository, adminUsers repository.IAdminUserRepository, tokens ITokenService) IAdminUserUsecase {
	return &adminUserUsecase{users: users, adminUsers: adminUsers, tokens: tokens}
}

func (u *adminUserUsecase) CreateAdmin(ctx context.Context, name, email, password string) (*entity.User, bool, error) {
	existing, err := u.users.FindByEmail(ctx, email)
	if err == nil {
		if existing.IsAdmin() {
			return existing, false, nil
		}
		return nil, false, entity.ErrEmailAlreadyExists
	}
	if !errors.Is(err, entity.ErrUserNotFound) {
		return nil, false, err
	}

	passwordHash, err := u.tokens.HashPassword(password)
	if err != nil {
		return nil, false, err
	}

	now := time.Now()
	admin := &entity.User{
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         entity.RoleAdmin,
		Status:       entity.StatusActive,
		TokenVersion: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := u.users.Create(ctx, admin); err != nil {
		if !errors.Is(err, entity.ErrEmailAlreadyExists) {
			return nil, false, err
		}

		existing, findErr := u.users.FindByEmail(ctx, email)
		if findErr != nil {
			return nil, false, findErr
		}
		if existing.IsAdmin() {
			return existing, false, nil
		}
		return nil, false, entity.ErrEmailAlreadyExists
	}

	return admin, true, nil
}

func (u *adminUserUsecase) ListUsers(ctx context.Context, actor *entity.User, input AdminUserListInput) (AdminUserListResult, error) {
	if err := validateAdminActor(actor); err != nil {
		return AdminUserListResult{}, err
	}

	var repositoryCursor *repository.AdminUserCursor
	if input.Cursor != nil {
		repositoryCursor = &repository.AdminUserCursor{
			CreatedAt: input.Cursor.CreatedAt,
			ID:        input.Cursor.ID,
		}
	}

	result, err := u.adminUsers.ListUsers(ctx, input.Limit, repositoryCursor)
	if err != nil {
		return AdminUserListResult{}, err
	}

	items := make([]AdminUserListItem, 0, len(result.Users))
	for _, user := range result.Users {
		items = append(items, AdminUserListItem{
			ID:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			Status:    user.Status,
			CreatedAt: user.CreatedAt,
		})
	}

	output := AdminUserListResult{
		Items:   items,
		HasMore: result.HasMore,
	}
	if result.HasMore && len(items) > 0 {
		cursor, err := encodeAdminUserCursor(AdminUserCursor{
			CreatedAt: result.LastCreatedAt,
			ID:        result.LastID,
		})
		if err != nil {
			return AdminUserListResult{}, err
		}
		output.NextCursor = &cursor
	}

	return output, nil
}

func (u *adminUserUsecase) GetUserDetail(ctx context.Context, actor *entity.User, targetUserID uint64) (AdminUserDetailResult, error) {
	if err := validateAdminActor(actor); err != nil {
		return AdminUserDetailResult{}, err
	}

	detail, err := u.adminUsers.FindUserDetail(ctx, targetUserID)
	if err != nil {
		return AdminUserDetailResult{}, err
	}

	videos := make([]AdminUserVideoItem, 0, len(detail.Videos))
	for _, video := range detail.Videos {
		videos = append(videos, AdminUserVideoItem{
			ID:               video.ID,
			Title:            video.Title,
			ProcessingStatus: video.ProcessingStatus,
			PublishStatus:    video.PublishStatus,
			CreatedAt:        video.CreatedAt,
		})
	}

	return AdminUserDetailResult{
		ID:        detail.ID,
		Name:      detail.Name,
		Email:     detail.Email,
		Status:    detail.Status,
		CreatedAt: detail.CreatedAt,
		Videos:    videos,
	}, nil
}

func (u *adminUserUsecase) SuspendUser(ctx context.Context, actor *entity.User, targetUserID uint64, reason string, requestID string) (AdminUserStatusResult, error) {
	if err := validateAdminOperation(actor, targetUserID, reason, requestID); err != nil {
		return AdminUserStatusResult{}, err
	}

	user, err := u.adminUsers.SuspendUser(ctx, actor.ID, targetUserID, reason, requestID, time.Now())
	if err != nil {
		return AdminUserStatusResult{}, err
	}

	return AdminUserStatusResult{ID: user.ID, Status: user.Status, UpdatedAt: user.UpdatedAt}, nil
}

func (u *adminUserUsecase) ResumeUser(ctx context.Context, actor *entity.User, targetUserID uint64, reason string, requestID string) (AdminUserStatusResult, error) {
	if err := validateAdminOperation(actor, targetUserID, reason, requestID); err != nil {
		return AdminUserStatusResult{}, err
	}

	user, err := u.adminUsers.ResumeUser(ctx, actor.ID, targetUserID, reason, requestID, time.Now())
	if err != nil {
		return AdminUserStatusResult{}, err
	}

	return AdminUserStatusResult{ID: user.ID, Status: user.Status, UpdatedAt: user.UpdatedAt}, nil
}

func encodeAdminUserCursor(cursor AdminUserCursor) (string, error) {
	payload, err := json.Marshal(AdminUserCursor{
		CreatedAt: cursor.CreatedAt,
		ID:        cursor.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func validateAdminActor(actor *entity.User) error {
	if actor == nil || !actor.IsActive() {
		return entity.ErrUnauthorized
	}
	if !actor.IsAdmin() {
		return entity.ErrAdminRequired
	}
	return nil
}

func validateAdminOperation(actor *entity.User, targetUserID uint64, reason, requestID string) error {
	if err := validateAdminActor(actor); err != nil {
		return err
	}
	if targetUserID == 0 || actor.ID == targetUserID {
		return entity.ErrUserManagementForbidden
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(requestID) == "" {
		return entity.ErrInvalidInput
	}
	return nil
}
