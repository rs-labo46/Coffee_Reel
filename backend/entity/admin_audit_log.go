package entity

import (
	"strings"
	"time"
	"unicode/utf8"
)

type AdminAuditTargetType string

const (
	AdminAuditTargetUser  AdminAuditTargetType = "user"
	AdminAuditTargetVideo AdminAuditTargetType = "video"
)

type AdminAuditAction string

const (
	AdminAuditActionUserSuspend               AdminAuditAction = "user_suspend"
	AdminAuditActionUserResume                AdminAuditAction = "user_resume"
	AdminAuditActionVideoHideByUserSuspension AdminAuditAction = "video_hide_by_user_suspension"
	AdminAuditActionVideoHide                 AdminAuditAction = "video_hide"
	AdminAuditActionVideoRestore              AdminAuditAction = "video_restore"
)

type AdminAuditLog struct {
	ID           uint64               `json:"id" gorm:"primaryKey"`
	AdminUserID  uint64               `json:"admin_user_id" gorm:"not null"`
	TargetType   AdminAuditTargetType `json:"target_type" gorm:"not null"`
	TargetID     uint64               `json:"target_id" gorm:"not null"`
	Action       AdminAuditAction     `json:"action" gorm:"not null"`
	BeforeStatus string               `json:"before_status" gorm:"not null"`
	AfterStatus  string               `json:"after_status" gorm:"not null"`
	Reason       string               `json:"reason" gorm:"type:varchar(500);not null"`
	RequestID    string               `json:"request_id" gorm:"not null"`
	CreatedAt    time.Time            `json:"created_at" gorm:"not null"`
}

func NewAdminAuditLog(
	adminUserID uint64,
	targetType AdminAuditTargetType,
	targetID uint64,
	action AdminAuditAction,
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
	if length := utf8.RuneCountInString(reason); length < 1 || length > 500 {
		return nil, ErrInvalidInput
	}
	if requestID == "" || !utf8.ValidString(requestID) {
		return nil, ErrInvalidInput
	}
	if !isValidAdminAuditTransition(targetType, action, beforeStatus, afterStatus) {
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
		CreatedAt:    now,
	}, nil
}

func isValidAdminAuditTransition(
	targetType AdminAuditTargetType,
	action AdminAuditAction,
	beforeStatus string,
	afterStatus string,
) bool {
	switch action {
	case AdminAuditActionUserSuspend:
		return targetType == AdminAuditTargetUser &&
			beforeStatus == string(StatusActive) &&
			afterStatus == string(StatusSuspended)
	case AdminAuditActionUserResume:
		return targetType == AdminAuditTargetUser &&
			beforeStatus == string(StatusSuspended) &&
			afterStatus == string(StatusActive)
	case AdminAuditActionVideoHideByUserSuspension:
		return targetType == AdminAuditTargetVideo &&
			beforeStatus == "published" &&
			afterStatus == "hidden"
	case AdminAuditActionVideoHide:
		return targetType == AdminAuditTargetVideo &&
			beforeStatus == string(VideoPublishPublished) &&
			afterStatus == string(VideoPublishHidden)

	case AdminAuditActionVideoRestore:
		return targetType == AdminAuditTargetVideo &&
			beforeStatus == string(VideoPublishHidden) &&
			afterStatus == string(VideoPublishPublished)
	default:
		return false
	}
}
