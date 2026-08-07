package entity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewAdminAuditLog(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))

	log, err := NewAdminAuditLog(
		1,
		AdminAuditTargetUser,
		2,
		AdminAuditActionUserSuspend,
		string(StatusActive),
		string(StatusSuspended),
		"  規約違反のため  ",
		"request-123",
		now,
	)
	if err != nil {
		t.Fatalf("NewAdminAuditLog() error = %v", err)
	}
	if log.Reason != "規約違反のため" {
		t.Fatalf("Reason = %q", log.Reason)
	}
	if !log.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want %s", log.CreatedAt, now)
	}
}

func TestNewAdminAuditLogAcceptsDefinedTransitions(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name         string
		targetType   AdminAuditTargetType
		action       AdminAuditAction
		beforeStatus string
		afterStatus  string
	}{
		{name: "user suspend", targetType: AdminAuditTargetUser, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended"},
		{name: "user resume", targetType: AdminAuditTargetUser, action: AdminAuditActionUserResume, beforeStatus: "suspended", afterStatus: "active"},
		{name: "video hide by suspension", targetType: AdminAuditTargetVideo, action: AdminAuditActionVideoHideByUserSuspension, beforeStatus: "published", afterStatus: "hidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewAdminAuditLog(1, tt.targetType, 2, tt.action, tt.beforeStatus, tt.afterStatus, "reason", "request", now); err != nil {
				t.Fatalf("NewAdminAuditLog() error = %v", err)
			}
		})
	}
}

func TestNewAdminAuditLogRejectsInvalidValues(t *testing.T) {
	now := time.Now()
	validReason := "reason"
	tooLongReason := strings.Repeat("あ", 501)

	tests := []struct {
		name         string
		adminUserID  uint64
		targetType   AdminAuditTargetType
		targetID     uint64
		action       AdminAuditAction
		beforeStatus string
		afterStatus  string
		reason       string
		requestID    string
		now          time.Time
	}{
		{name: "zero admin ID", targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended", reason: validReason, requestID: "request", now: now},
		{name: "zero target ID", adminUserID: 1, targetType: AdminAuditTargetUser, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended", reason: validReason, requestID: "request", now: now},
		{name: "blank reason", adminUserID: 1, targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended", reason: "   ", requestID: "request", now: now},
		{name: "reason too long", adminUserID: 1, targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended", reason: tooLongReason, requestID: "request", now: now},
		{name: "blank request ID", adminUserID: 1, targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended", reason: validReason, requestID: " ", now: now},
		{name: "invalid transition", adminUserID: 1, targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditActionUserSuspend, beforeStatus: "suspended", afterStatus: "active", reason: validReason, requestID: "request", now: now},
		{name: "unknown action", adminUserID: 1, targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditAction("unknown"), beforeStatus: "active", afterStatus: "suspended", reason: validReason, requestID: "request", now: now},
		{name: "zero time", adminUserID: 1, targetType: AdminAuditTargetUser, targetID: 2, action: AdminAuditActionUserSuspend, beforeStatus: "active", afterStatus: "suspended", reason: validReason, requestID: "request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := NewAdminAuditLog(tt.adminUserID, tt.targetType, tt.targetID, tt.action, tt.beforeStatus, tt.afterStatus, tt.reason, tt.requestID, tt.now)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if log != nil {
				t.Fatalf("log = %#v, want nil", log)
			}
		})
	}
}
