package entity

import (
	"strings"
	"time"
	"unicode/utf8"
)

type AuditTargetType string

const (
	AuditTargetUser  AuditTargetType = "user"
	AuditTargetVideo AuditTargetType = "video"
)

type AuditAction string

const (
	AuditActionUserSuspend           AuditAction = "user_suspend"
	AuditActionUserResume            AuditAction = "user_resume"
	AuditActionVideoHide             AuditAction = "video_hide"
	AuditActionVideoRestore          AuditAction = "video_restore"
	AuditActionVideoHideBySuspension AuditAction = "video_hide_by_user_suspension"
)

type AdminAuditLog struct {
	ID           uint64          `json:"id" gorm:"primaryKey"`
	AdminUserID  uint64          `json:"admin_user_id" gorm:"not null"`
	TargetType   AuditTargetType `json:"target_type" gorm:"not null"`
	TargetID     uint64          `json:"target_id" gorm:"not null"`
	Action       AuditAction     `json:"action" gorm:"not null"`
	BeforeStatus string          `json:"before_status" gorm:"not null"`
	AfterStatus  string          `json:"after_status" gorm:"not null"`
	Reason       string          `json:"reason" gorm:"type:varchar(500);not null"`
	RequestID    string          `json:"request_id" gorm:"not null"`
	CreatedAt    time.Time       `json:"created_at" gorm:"not null"`
}

func NewAdminAuditLog(
	adminUserID uint64,
	targetType AuditTargetType,
	targetID uint64,
	action AuditAction,
	beforeStatus string,
	afterStatus string,
	reason string,
	requestID string,
	now time.Time,
) (*AdminAuditLog, error) {
	reason = strings.TrimSpace(reason)
	requestID = strings.TrimSpace(requestID)

	if adminUserID == 0 || targetID == 0 || now.IsZero() {
		return nil, ErrInvalidInput
	}

	if !utf8.ValidString(reason) {
		return nil, ErrInvalidInput
	}

	reasonLength := utf8.RuneCountInString(reason)
	if reasonLength < 1 || reasonLength > 500 {
		return nil, ErrInvalidInput
	}

	if requestID == "" || !utf8.ValidString(requestID) {
		return nil, ErrInvalidInput
	}

	validTransition := false

	switch action {
	case AuditActionUserSuspend:
		validTransition = targetType == AuditTargetUser &&
			beforeStatus == string(StatusActive) &&
			afterStatus == string(StatusSuspended)

	case AuditActionUserResume:
		validTransition = targetType == AuditTargetUser &&
			beforeStatus == string(StatusSuspended) &&
			afterStatus == string(StatusActive)

	case AuditActionVideoHide:
		validTransition = targetType == AuditTargetVideo &&
			beforeStatus == "published" &&
			afterStatus == "hidden"

	case AuditActionVideoRestore:
		validTransition = targetType == AuditTargetVideo &&
			beforeStatus == "hidden" &&
			afterStatus == "published"

	case AuditActionVideoHideBySuspension:
		validTransition = targetType == AuditTargetVideo &&
			beforeStatus == "published" &&
			afterStatus == "hidden"
	}

	if !validTransition {
		return nil, ErrInvalidInput
	}

	return &AdminAuditLog{
		AdminUserID:  adminUserID,
		TargetType:   targetType,
		TargetID:     targetID,
		Action:       action,
		BeforeStatus: beforeStatus,
		AfterStatus:  afterStatus,
		Reason:       reason,
		RequestID:    requestID,
		CreatedAt:    now.UTC(),
	}, nil
}
