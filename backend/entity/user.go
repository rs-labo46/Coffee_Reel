package entity

import "time"

type UserRole string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"
)

type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusSuspended UserStatus = "suspended"
)

type User struct {
	ID           uint64     `json:"id" gorm:"primaryKey"`
	Name         string     `json:"name" gorm:"not null"`
	Email        string     `json:"email" gorm:"not null;uniqueIndex"`
	PasswordHash string     `json:"-" gorm:"not null"`
	Role         UserRole   `json:"role" gorm:"not null"`
	Status       UserStatus `json:"status" gorm:"not null"`
	TokenVersion uint64     `json:"-" gorm:"not null;default:0"`
	CreatedAt    time.Time  `json:"created_at" gorm:"not null"`
	UpdatedAt    time.Time  `json:"updated_at" gorm:"not null"`
}

func (u User) IsActive() bool {
	return u.Status == StatusActive
}
func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) InvalidateAccessTokens(now time.Time) {
	u.TokenVersion++
	u.UpdatedAt = now.UTC()
}
