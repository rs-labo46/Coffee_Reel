package entity

import (
	"errors"
	"testing"
	"time"
)

func TestNewAdminAuditLogAcceptsAdminVideoTransitions(t *testing.T) {
	now := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		action       AdminAuditAction
		beforeStatus string
		afterStatus  string
	}{
		{
			name:         "hide",
			action:       AdminAuditActionVideoHide,
			beforeStatus: string(VideoPublishPublished),
			afterStatus:  string(VideoPublishHidden),
		},
		{
			name:         "restore",
			action:       AdminAuditActionVideoRestore,
			beforeStatus: string(VideoPublishHidden),
			afterStatus:  string(VideoPublishPublished),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := NewAdminAuditLog(
				1,
				AdminAuditTargetVideo,
				2,
				tt.action,
				tt.beforeStatus,
				tt.afterStatus,
				"  管理者確認理由  ",
				"request-admin-video",
				now,
			)
			if err != nil {
				t.Fatalf("NewAdminAuditLog() error = %v", err)
			}
			if log.Action != tt.action ||
				log.TargetType != AdminAuditTargetVideo ||
				log.BeforeStatus != tt.beforeStatus ||
				log.AfterStatus != tt.afterStatus {
				t.Fatalf("log = %#v", log)
			}
			if log.Reason != "管理者確認理由" {
				t.Fatalf("Reason = %q", log.Reason)
			}
		})
	}
}

func TestNewAdminAuditLogRejectsInvalidAdminVideoTransitions(t *testing.T) {
	now := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		action       AdminAuditAction
		beforeStatus string
		afterStatus  string
	}{
		{
			name:         "hide from private",
			action:       AdminAuditActionVideoHide,
			beforeStatus: string(VideoPublishPrivate),
			afterStatus:  string(VideoPublishHidden),
		},
		{
			name:         "restore from private",
			action:       AdminAuditActionVideoRestore,
			beforeStatus: string(VideoPublishPrivate),
			afterStatus:  string(VideoPublishPublished),
		},
		{
			name:         "restore to hidden",
			action:       AdminAuditActionVideoRestore,
			beforeStatus: string(VideoPublishHidden),
			afterStatus:  string(VideoPublishHidden),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := NewAdminAuditLog(
				1,
				AdminAuditTargetVideo,
				2,
				tt.action,
				tt.beforeStatus,
				tt.afterStatus,
				"reason",
				"request-admin-video",
				now,
			)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if log != nil {
				t.Fatalf("log = %#v, want nil", log)
			}
		})
	}
}
