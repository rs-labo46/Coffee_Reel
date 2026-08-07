package repository

import (
	"coffee-reel/entity"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IAdminUserRepository interface {
	ListUsers(ctx context.Context, limit int, cursor *AdminUserCursor) (AdminUserListResult, error)
	FindUserDetail(ctx context.Context, userID uint64) (*AdminUserDetail, error)
	SuspendUser(ctx context.Context, adminUserID, targetUserID uint64, reason, requestID string, now time.Time) (*entity.User, error)
	ResumeUser(ctx context.Context, adminUserID, targetUserID uint64, reason, requestID string, now time.Time) (*entity.User, error)
}

type AdminUserCursor struct {
	CreatedAt time.Time
	ID        uint64
}

type AdminUserListItem struct {
	ID        uint64
	Name      string
	Email     string
	Status    entity.UserStatus
	CreatedAt time.Time
}

type AdminUserListResult struct {
	Users         []AdminUserListItem
	HasMore       bool
	LastCreatedAt time.Time
	LastID        uint64
}

type AdminUserVideoItem struct {
	ID               uint64
	Title            string
	ProcessingStatus string
	PublishStatus    string
	CreatedAt        time.Time
}

type AdminUserDetail struct {
	ID        uint64
	Name      string
	Email     string
	Status    entity.UserStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	Videos    []AdminUserVideoItem
}

type adminUserRepository struct {
	db *gorm.DB
}

func NewAdminUserRepository(db *gorm.DB) IAdminUserRepository {
	return &adminUserRepository{db: db}
}

func (r *adminUserRepository) ListUsers(ctx context.Context, limit int, cursor *AdminUserCursor) (AdminUserListResult, error) {
	users := make([]AdminUserListItem, 0, limit+1)
	query := r.db.WithContext(ctx).
		Model(&entity.User{}).
		Select("id", "name", "email", "status", "created_at").
		Where("role = ?", entity.RoleUser).
		Where("status IN ?", []entity.UserStatus{entity.StatusActive, entity.StatusSuspended})

	if cursor != nil {
		query = query.Where(
			"created_at < ? OR (created_at = ? AND id < ?)",
			cursor.CreatedAt,
			cursor.CreatedAt,
			cursor.ID,
		)
	}

	if err := query.
		Order("created_at DESC").
		Order("id DESC").
		Limit(limit + 1).
		Scan(&users).Error; err != nil {
		return AdminUserListResult{}, fmt.Errorf("list admin users: %w", err)
	}

	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}

	result := AdminUserListResult{Users: users, HasMore: hasMore}
	if len(users) > 0 {
		last := users[len(users)-1]
		result.LastCreatedAt = last.CreatedAt
		result.LastID = last.ID
	}
	return result, nil
}

func (r *adminUserRepository) FindUserDetail(ctx context.Context, userID uint64) (*AdminUserDetail, error) {
	var detail AdminUserDetail
	if err := r.db.WithContext(ctx).
		Model(&entity.User{}).
		Select("id", "name", "email", "status", "created_at", "updated_at").
		Where("id = ? AND role = ?", userID, entity.RoleUser).
		Take(&detail).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrUserNotFound
		}
		return nil, fmt.Errorf("find admin user detail: %w", err)
	}

	videos := make([]AdminUserVideoItem, 0)
	if err := r.db.WithContext(ctx).
		Model(&entity.Video{}).
		Select("id", "title", "processing_status", "publish_status", "created_at").
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Order("id DESC").
		Scan(&videos).Error; err != nil {
		return nil, fmt.Errorf("list admin user videos: %w", err)
	}

	detail.Videos = videos
	return &detail, nil
}

func (r *adminUserRepository) SuspendUser(
	ctx context.Context,
	adminUserID uint64,
	targetUserID uint64,
	reason string,
	requestID string,
	now time.Time,
) (*entity.User, error) {
	var updatedUser entity.User

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", targetUserID).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrUserNotFound
			}
			return fmt.Errorf("lock user for suspension: %w", err)
		}

		beforeStatus := user.Status
		if err := user.Suspend(now); err != nil {
			return err
		}

		result := tx.Model(&entity.User{}).
			Where("id = ?", user.ID).
			Select("status", "token_version", "updated_at").
			Updates(&user)
		if result.Error != nil {
			return fmt.Errorf("save suspended user: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return entity.ErrUserStatusConflict
		}

		if err := tx.Model(&entity.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", now).Error; err != nil {
			return fmt.Errorf("revoke refresh tokens for suspended user: %w", err)
		}

		userAudit, err := entity.NewAdminAuditLog(
			adminUserID,
			entity.AdminAuditTargetUser,
			user.ID,
			entity.AdminAuditActionUserSuspend,
			string(beforeStatus),
			string(user.Status),
			reason,
			requestID,
			now,
		)
		if err != nil {
			return err
		}
		if err := tx.Create(userAudit).Error; err != nil {
			return fmt.Errorf("create user suspension audit log: %w", err)
		}

		if err := hidePublishedVideosForSuspension(tx, adminUserID, user.ID, reason, requestID, now); err != nil {
			return err
		}

		updatedUser = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updatedUser, nil
}

func hidePublishedVideosForSuspension(
	tx *gorm.DB,
	adminUserID uint64,
	userID uint64,
	reason string,
	requestID string,
	now time.Time,
) error {
	videos := make([]entity.Video, 0)
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"user_id = ? AND processing_status = ? AND publish_status = ? AND deleted_at IS NULL",
			userID,
			entity.VideoProcessingReady,
			entity.VideoPublishPublished,
		).
		Order("id ASC").
		Find(&videos).Error; err != nil {
		return fmt.Errorf("lock published videos for suspended user: %w", err)
	}

	for index := range videos {
		video := &videos[index]
		beforeStatus := video.PublishStatus
		if err := video.HideByAdmin(now); err != nil {
			return err
		}

		result := tx.Model(&entity.Video{}).
			Where(
				"id = ? AND processing_status = ? AND publish_status = ? AND deleted_at IS NULL",
				video.ID,
				entity.VideoProcessingReady,
				beforeStatus,
			).
			Select("publish_status", "updated_at").
			Updates(video)
		if result.Error != nil {
			return fmt.Errorf("hide suspended user video: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return entity.ErrVideoStateConflict
		}

		audit, err := entity.NewAdminAuditLog(
			adminUserID,
			entity.AdminAuditTargetVideo,
			video.ID,
			entity.AdminAuditActionVideoHideByUserSuspension,
			string(beforeStatus),
			string(video.PublishStatus),
			reason,
			requestID,
			now,
		)
		if err != nil {
			return err
		}
		if err := tx.Create(audit).Error; err != nil {
			return fmt.Errorf("create suspended user video audit log: %w", err)
		}
	}
	return nil
}

func (r *adminUserRepository) ResumeUser(
	ctx context.Context,
	adminUserID uint64,
	targetUserID uint64,
	reason string,
	requestID string,
	now time.Time,
) (*entity.User, error) {
	var updatedUser entity.User

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", targetUserID).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrUserNotFound
			}
			return fmt.Errorf("lock user for resume: %w", err)
		}

		beforeStatus := user.Status
		if err := user.Resume(now); err != nil {
			return err
		}

		result := tx.Model(&entity.User{}).
			Where("id = ?", user.ID).
			Select("status", "updated_at").
			Updates(&user)
		if result.Error != nil {
			return fmt.Errorf("save resumed user: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return entity.ErrUserStatusConflict
		}

		auditLog, err := entity.NewAdminAuditLog(
			adminUserID,
			entity.AdminAuditTargetUser,
			user.ID,
			entity.AdminAuditActionUserResume,
			string(beforeStatus),
			string(user.Status),
			reason,
			requestID,
			now,
		)
		if err != nil {
			return err
		}
		if err := tx.Create(auditLog).Error; err != nil {
			return fmt.Errorf("create user resume audit log: %w", err)
		}

		updatedUser = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updatedUser, nil
}
