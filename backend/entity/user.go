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
	ID           uint64     `json:"id" gorm:"primaryKey;index:idx_users_admin_list,sort:desc,priority:3"`
	Name         string     `json:"name" gorm:"not null"`
	Email        string     `json:"email" gorm:"not null;uniqueIndex:uq_users_email"`
	PasswordHash string     `json:"-" gorm:"not null"`
	Role         UserRole   `json:"role" gorm:"not null;index:idx_users_admin_list,priority:1"`
	Status       UserStatus `json:"status" gorm:"not null"`
	TokenVersion uint64     `json:"-" gorm:"not null;default:0"`
	CreatedAt    time.Time  `json:"created_at" gorm:"not null;index:idx_users_admin_list,sort:desc,priority:2"`
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

func (u *User) Suspend(now time.Time) error {
	if u.Role != RoleUser {
		return ErrUserManagementForbidden
	}

	if u.Status != StatusActive {
		return ErrUserStatusConflict
	}

	u.Status = StatusSuspended
	u.TokenVersion++
	u.UpdatedAt = now.UTC()

	return nil
}

func (u *User) Resume(now time.Time) error {
	if u.Role != RoleUser {
		return ErrUserManagementForbidden
	}
	if u.Status != StatusSuspended {
		return ErrUserStatusConflict
	}
	u.Status = StatusActive
	u.UpdatedAt = now.UTC()

	return nil
}
