package entity

import "time"

type IdempotencyScope string

const IdempotencyScopeVideoCreate IdempotencyScope = "video_create"

type IdempotencyRecord struct {
	ID          uint64           `json:"id" gorm:"primaryKey"`
	UserID      uint64           `json:"user_id" gorm:"not null;uniqueIndex:uq_idempotency_records_user_scope_key,priority:1"`
	Scope       IdempotencyScope `json:"scope" gorm:"type:varchar(64);not null;uniqueIndex:uq_idempotency_records_user_scope_key,priority:2"`
	KeyHash     string           `json:"-" gorm:"type:char(64);not null;uniqueIndex:uq_idempotency_records_user_scope_key,priority:3"`
	RequestHash string           `json:"-" gorm:"type:char(64);not null"`
	ResourceID  uint64           `json:"resource_id" gorm:"not null"`
	ExpiresAt   time.Time        `json:"expires_at" gorm:"not null;index:idx_idempotency_records_expires_at"`
	CreatedAt   time.Time        `json:"created_at" gorm:"not null"`
}

func NewIdempotencyRecord(userID uint64, keyHash, requestHash string, expiresAt, now time.Time) (*IdempotencyRecord, error) {
	if userID == 0 || len(keyHash) != 64 || len(requestHash) != 64 || now.IsZero() || !expiresAt.After(now) {
		return nil, ErrInvalidInput
	}
	return &IdempotencyRecord{
		UserID:      userID,
		Scope:       IdempotencyScopeVideoCreate,
		KeyHash:     keyHash,
		RequestHash: requestHash,
		ExpiresAt:   expiresAt.UTC(),
		CreatedAt:   now.UTC(),
	}, nil
}
