package entity

import "time"

type RefreshToken struct {
	ID           uint64        `json:"-" gorm:"primaryKey"`
	UserID       uint64        `json:"-" gorm:"not null;index"`
	User         User          `json:"-" gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	TokenHash    string        `json:"-" gorm:"not null;uniqueIndex"`
	FamilyID     string        `json:"-" gorm:"not null;index"`
	ReplacedByID *uint64       `json:"-" gorm:"index"`
	ReplacedBy   *RefreshToken `json:"-" gorm:"foreignKey:ReplacedByID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ExpiresAt    time.Time     `json:"-" gorm:"not null;index"`
	UsedAt       *time.Time    `json:"-"`
	RevokedAt    *time.Time    `json:"-"`
	CreatedAt    time.Time     `json:"-" gorm:"not null"`
}

func (r RefreshToken) IsExpired(now time.Time) bool {
	return !now.UTC().Before(r.ExpiresAt.UTC())
}

func (r RefreshToken) IsUsable(now time.Time) bool {
	return r.UsedAt == nil && r.RevokedAt == nil && !r.IsExpired(now)
}

func (r *RefreshToken) MarkUsed(replacedByID uint64, now time.Time) {
	usedAt := now.UTC()
	r.UsedAt = &usedAt
	r.ReplacedByID = &replacedByID
}

func (r *RefreshToken) Revoke(now time.Time) {
	if r.RevokedAt != nil {
		return
	}
	revokedAt := now.UTC()
	r.RevokedAt = &revokedAt
}
