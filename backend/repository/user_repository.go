package repository

import (
	"coffee-reel/entity"
	"context"

	"gorm.io/gorm"
)

const postgresUniqueViolation = "23505"

type UserRepository interface {
	Create(ctx context.Context, user *entity.User) error
	FindByEmail(ctx context.Context, email string) error
	FindByID(ctx context.Context, id uint64) (entity.User, error)
	Update(ctx context.Context, user *entity.User) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db}
}

//Userを作成

//EmailからUserを取得

//UserIDからUserを取得

//Userの状態やTokenVersionを更新
