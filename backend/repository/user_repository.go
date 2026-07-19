package repository

import (
	"coffee-reel/entity"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const postgresUniqueViolation = "23505"

type IUserRepository interface {
	Create(ctx context.Context, user *entity.User) error
	FindByEmail(ctx context.Context, email string) (*entity.User, error)
	FindByID(ctx context.Context, id uint64) (*entity.User, error)
	Update(ctx context.Context, user *entity.User) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) IUserRepository {
	return &userRepository{db}
}

// Userを作成
func (r *userRepository) Create(ctx context.Context, user *entity.User) error {
	if user == nil {
		return fmt.Errorf("user is required")
	}
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return entity.ErrEmailAlreadyExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// EmailからUserを取得
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User

	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return &user, nil
}

// UserIDからUserを取得
func (r *userRepository) FindByID(ctx context.Context, id uint64) (*entity.User, error) {
	var user entity.User

	if err := r.db.WithContext(ctx).Where("id=?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &user, nil
}

// Userの状態やTokenVersionを更新
func (r *userRepository) Update(ctx context.Context, user *entity.User) error {
	if user == nil {
		return fmt.Errorf("user is required")
	}
	result := r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", user.ID).Select("status", "token_version", "updated_at").Updates(user)

	if result.Error != nil {
		return fmt.Errorf("update user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return entity.ErrUserNotFound
	}
	return nil
}
