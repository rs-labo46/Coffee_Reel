//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/migrate"

	"gorm.io/gorm"
)

func openAdminVideoIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openPostgresIntegrationDB(t)

	if err := migrate.MigrateVideos(db); err != nil {
		t.Fatalf("MigrateVideos: %v", err)
	}
	if err := migrate.MigrateVideoOutputMetas(db); err != nil {
		t.Fatalf("MigrateVideoOutputMetas: %v", err)
	}
	if err := migrate.MigrateAdminAuditLogVideoActions(db); err != nil {
		t.Fatalf("MigrateAdminAuditLogVideoActions: %v", err)
	}

	return db
}

func createAdminVideoIntegrationUser(
	t *testing.T,
	db *gorm.DB,
	name string,
	email string,
	role entity.UserRole,
	status entity.UserStatus,
	createdAt time.Time,
) *entity.User {
	t.Helper()

	user := &entity.User{
		Name:         name,
		Email:        email,
		PasswordHash: "hashed-password",
		Role:         role,
		Status:       status,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createAdminVideoIntegrationVideo(
	t *testing.T,
	db *gorm.DB,
	ownerID uint64,
	keySuffix string,
	title string,
	processingStatus entity.VideoProcessingStatus,
	publishStatus entity.VideoPublishStatus,
	createdAt time.Time,
	deletedAt *time.Time,
) *entity.Video {
	t.Helper()

	updatedAt := createdAt
	var normalizedDeletedAt *time.Time
	if deletedAt != nil {
		value := *deletedAt
		normalizedDeletedAt = &value
		updatedAt = value
	}

	video := &entity.Video{
		UserID:            ownerID,
		Category:          entity.CategoryBrewing,
		Title:             title,
		Description:       "description",
		OriginalObjectKey: fmt.Sprintf("videos/%d/source/%s.mp4", ownerID, keySuffix),
		UploadExpiresAt:   createdAt.Add(15 * time.Minute),
		ProcessingStatus:  processingStatus,
		PublishStatus:     publishStatus,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		DeletedAt:         normalizedDeletedAt,
	}
	if err := db.Create(video).Error; err != nil {
		t.Fatalf("create video: %v", err)
	}
	return video
}

func createAdminVideoIntegrationOutputMeta(
	t *testing.T,
	db *gorm.DB,
	videoID uint64,
	keySuffix string,
	createdAt time.Time,
) *entity.OutputVideoMeta {
	t.Helper()

	meta := &entity.OutputVideoMeta{
		VideoID:            videoID,
		VideoObjectKey:     fmt.Sprintf("videos/%d/output/%s.mp4", videoID, keySuffix),
		ThumbnailObjectKey: fmt.Sprintf("videos/%d/thumbnail/%s.jpg", videoID, keySuffix),
		Container:          "mp4",
		Width:              720,
		Height:             1280,
		FrameRate:          30,
		VideoCodec:         "h264",
		HasAudio:           true,
		AudioCodec:         "aac",
		CreatedAt:          createdAt,
	}
	if err := db.Create(meta).Error; err != nil {
		t.Fatalf("create output meta: %v", err)
	}
	return meta
}

func TestAdminVideoRepositoryListUsesCursorAndExcludesDeleted(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Owner",
		"owner-list@example.com",
		entity.RoleUser,
		entity.StatusActive,
		time.Date(2026, 8, 6, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
	)

	oldest := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"list-oldest",
		"oldest",
		entity.VideoProcessingUploading,
		entity.VideoPublishPrivate,
		time.Date(2026, 8, 6, 1, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		nil,
	)
	sameTimeLowerID := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"list-same-lower",
		"same lower",
		entity.VideoProcessingReady,
		entity.VideoPublishHidden,
		time.Date(2026, 8, 6, 2, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		nil,
	)
	sameTimeHigherID := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"list-same-higher",
		"same higher",
		entity.VideoProcessingReady,
		entity.VideoPublishPublished,
		time.Date(2026, 8, 6, 2, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		nil,
	)
	newest := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"list-newest",
		"newest",
		entity.VideoProcessingFailed,
		entity.VideoPublishPrivate,
		time.Date(2026, 8, 6, 3, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		nil,
	)
	deletedAt := time.Date(2026, 8, 6, 4, 1, 0, 0, time.FixedZone("JST", 9*60*60))
	deleted := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"list-deleted",
		"deleted",
		entity.VideoProcessingReady,
		entity.VideoPublishPrivate,
		time.Date(2026, 8, 6, 4, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		&deletedAt,
	)

	first, err := repository.List(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("List first page: %v", err)
	}
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first page = %#v", first)
	}
	if first.Items[0].VideoID != newest.ID ||
		first.Items[1].VideoID != sameTimeHigherID.ID {
		t.Fatalf("first IDs = %d, %d", first.Items[0].VideoID, first.Items[1].VideoID)
	}
	if first.Items[0].AuthorID != owner.ID ||
		first.Items[0].AuthorName != owner.Name ||
		first.Items[0].AuthorStatus != owner.Status {
		t.Fatalf("author = %#v", first.Items[0])
	}
	if first.LastID != sameTimeHigherID.ID ||
		!first.LastCreatedAt.Equal(sameTimeHigherID.CreatedAt) {
		t.Fatalf("first cursor = %#v", first)
	}

	second, err := repository.List(
		context.Background(),
		2,
		&AdminVideoCursor{
			CreatedAt: first.LastCreatedAt,
			ID:        first.LastID,
		},
	)
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}
	if len(second.Items) != 2 || second.HasMore {
		t.Fatalf("second page = %#v", second)
	}
	if second.Items[0].VideoID != sameTimeLowerID.ID ||
		second.Items[1].VideoID != oldest.ID {
		t.Fatalf("second IDs = %d, %d", second.Items[0].VideoID, second.Items[1].VideoID)
	}

	for _, page := range []AdminVideoListResult{first, second} {
		for _, item := range page.Items {
			if item.VideoID == deleted.ID {
				t.Fatalf("deleted video %d was returned", deleted.ID)
			}
		}
	}
}

func TestAdminVideoRepositoryFindByIDReturnsOutputMetaAndExcludesDeleted(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	now := time.Date(2026, 8, 6, 1, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Owner",
		"owner-detail@example.com",
		entity.RoleUser,
		entity.StatusSuspended,
		now,
	)
	video := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"detail",
		"detail title",
		entity.VideoProcessingReady,
		entity.VideoPublishHidden,
		now,
		nil,
	)
	meta := createAdminVideoIntegrationOutputMeta(t, db, video.ID, "detail", now)

	detail, err := repository.FindByID(context.Background(), video.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if detail.VideoID != video.ID ||
		detail.AuthorID != owner.ID ||
		detail.AuthorName != owner.Name ||
		detail.AuthorStatus != entity.StatusSuspended ||
		detail.OutputMeta == nil ||
		detail.OutputMeta.VideoObjectKey != meta.VideoObjectKey ||
		detail.OutputMeta.ThumbnailObjectKey != meta.ThumbnailObjectKey {
		t.Fatalf("detail = %#v", detail)
	}

	deletedAt := now.Add(time.Minute)
	deleted := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"detail-deleted",
		"deleted detail",
		entity.VideoProcessingReady,
		entity.VideoPublishPrivate,
		now,
		&deletedAt,
	)
	if _, err := repository.FindByID(context.Background(), deleted.ID); !errors.Is(err, entity.ErrVideoNotFound) {
		t.Fatalf("deleted error = %v, want ErrVideoNotFound", err)
	}
}

func TestAdminVideoRepositoryHideCommitsVideoAndAuditLog(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	admin := createAdminVideoIntegrationUser(
		t,
		db,
		"Admin",
		"admin-hide@example.com",
		entity.RoleAdmin,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Owner",
		"owner-hide@example.com",
		entity.RoleUser,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	video := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"hide",
		"hide title",
		entity.VideoProcessingReady,
		entity.VideoPublishPublished,
		now.Add(-30*time.Minute),
		nil,
	)

	result, err := repository.Hide(
		context.Background(),
		admin.ID,
		video.ID,
		"規約違反",
		"request-hide-integration",
		now,
	)
	if err != nil {
		t.Fatalf("Hide: %v", err)
	}
	if result.PublishStatus != entity.VideoPublishHidden ||
		result.ProcessingStatus != entity.VideoProcessingReady ||
		!result.UpdatedAt.Equal(now) {
		t.Fatalf("result = %#v", result)
	}

	var storedVideo entity.Video
	if err := db.First(&storedVideo, video.ID).Error; err != nil {
		t.Fatalf("load video: %v", err)
	}
	if storedVideo.PublishStatus != entity.VideoPublishHidden ||
		storedVideo.ProcessingStatus != entity.VideoProcessingReady ||
		!storedVideo.UpdatedAt.Equal(now) {
		t.Fatalf("stored video = %#v", storedVideo)
	}

	var audit entity.AdminAuditLog
	if err := db.Where("request_id = ?", "request-hide-integration").Take(&audit).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if audit.AdminUserID != admin.ID ||
		audit.TargetType != entity.AdminAuditTargetVideo ||
		audit.TargetID != video.ID ||
		audit.Action != entity.AdminAuditActionVideoHide ||
		audit.BeforeStatus != string(entity.VideoPublishPublished) ||
		audit.AfterStatus != string(entity.VideoPublishHidden) ||
		audit.Reason != "規約違反" ||
		!audit.CreatedAt.Equal(now) {
		t.Fatalf("audit = %#v", audit)
	}
}

func TestAdminVideoRepositoryHideRollsBackWhenAuditInsertFails(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	admin := createAdminVideoIntegrationUser(
		t,
		db,
		"Admin",
		"admin-hide-rollback@example.com",
		entity.RoleAdmin,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Owner",
		"owner-hide-rollback@example.com",
		entity.RoleUser,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	video := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"hide-rollback",
		"hide rollback",
		entity.VideoProcessingReady,
		entity.VideoPublishPublished,
		now.Add(-30*time.Minute),
		nil,
	)

	if err := db.Exec(`
ALTER TABLE admin_audit_logs
ADD CONSTRAINT chk_test_reject_admin_video_audit
CHECK (request_id <> 'reject-audit')
`).Error; err != nil {
		t.Fatalf("add rejecting audit constraint: %v", err)
	}

	result, err := repository.Hide(
		context.Background(),
		admin.ID,
		video.ID,
		"規約違反",
		"reject-audit",
		now,
	)
	if err == nil {
		t.Fatal("Hide error = nil, want audit insertion failure")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}

	var storedVideo entity.Video
	if err := db.First(&storedVideo, video.ID).Error; err != nil {
		t.Fatalf("load video: %v", err)
	}
	if storedVideo.PublishStatus != entity.VideoPublishPublished {
		t.Fatalf("publish status = %s, want published", storedVideo.PublishStatus)
	}

	var auditCount int64
	if err := db.Model(&entity.AdminAuditLog{}).
		Where("request_id = ?", "reject-audit").
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("audit count = %d, want 0", auditCount)
	}
}

func TestAdminVideoRepositoryRestoreCommitsVideoAndAuditLog(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	now := time.Date(2026, 8, 6, 3, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	admin := createAdminVideoIntegrationUser(
		t,
		db,
		"Admin",
		"admin-restore@example.com",
		entity.RoleAdmin,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Owner",
		"owner-restore@example.com",
		entity.RoleUser,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	video := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"restore",
		"restore title",
		entity.VideoProcessingReady,
		entity.VideoPublishHidden,
		now.Add(-30*time.Minute),
		nil,
	)

	result, err := repository.Restore(
		context.Background(),
		admin.ID,
		video.ID,
		"再確認済み",
		"request-restore-integration",
		now,
	)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.PublishStatus != entity.VideoPublishPublished ||
		result.ProcessingStatus != entity.VideoProcessingReady ||
		!result.UpdatedAt.Equal(now) {
		t.Fatalf("result = %#v", result)
	}

	var storedVideo entity.Video
	if err := db.First(&storedVideo, video.ID).Error; err != nil {
		t.Fatalf("load video: %v", err)
	}
	if storedVideo.PublishStatus != entity.VideoPublishPublished ||
		storedVideo.ProcessingStatus != entity.VideoProcessingReady ||
		!storedVideo.UpdatedAt.Equal(now) {
		t.Fatalf("stored video = %#v", storedVideo)
	}

	var audit entity.AdminAuditLog
	if err := db.Where("request_id = ?", "request-restore-integration").Take(&audit).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if audit.Action != entity.AdminAuditActionVideoRestore ||
		audit.BeforeStatus != string(entity.VideoPublishHidden) ||
		audit.AfterStatus != string(entity.VideoPublishPublished) ||
		audit.TargetID != video.ID ||
		audit.AdminUserID != admin.ID ||
		audit.Reason != "再確認済み" ||
		audit.RequestID != "request-restore-integration" ||
		!audit.CreatedAt.Equal(now) {
		t.Fatalf("audit = %#v", audit)
	}
}

func TestAdminVideoRepositoryRestoreRejectsSuspendedOwner(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	now := time.Date(2026, 8, 6, 4, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	admin := createAdminVideoIntegrationUser(
		t,
		db,
		"Admin",
		"admin-restore-suspended@example.com",
		entity.RoleAdmin,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Suspended Owner",
		"owner-restore-suspended@example.com",
		entity.RoleUser,
		entity.StatusSuspended,
		now.Add(-time.Hour),
	)
	video := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"restore-suspended",
		"restore suspended",
		entity.VideoProcessingReady,
		entity.VideoPublishHidden,
		now.Add(-30*time.Minute),
		nil,
	)

	result, err := repository.Restore(
		context.Background(),
		admin.ID,
		video.ID,
		"公開再開確認",
		"request-restore-suspended",
		now,
	)
	if !errors.Is(err, entity.ErrUserSuspended) {
		t.Fatalf("error = %v, want ErrUserSuspended", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}

	var storedVideo entity.Video
	if err := db.First(&storedVideo, video.ID).Error; err != nil {
		t.Fatalf("load video: %v", err)
	}
	if storedVideo.PublishStatus != entity.VideoPublishHidden {
		t.Fatalf("publish status = %s, want hidden", storedVideo.PublishStatus)
	}

	var auditCount int64
	if err := db.Model(&entity.AdminAuditLog{}).
		Where("request_id = ?", "request-restore-suspended").
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("audit count = %d, want 0", auditCount)
	}
}

func TestAdminVideoRepositoryHideRejectsInvalidStateWithoutAudit(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	repository := NewAdminVideoRepository(db)
	now := time.Date(2026, 8, 6, 5, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	admin := createAdminVideoIntegrationUser(
		t,
		db,
		"Admin",
		"admin-hide-invalid@example.com",
		entity.RoleAdmin,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	owner := createAdminVideoIntegrationUser(
		t,
		db,
		"Owner",
		"owner-hide-invalid@example.com",
		entity.RoleUser,
		entity.StatusActive,
		now.Add(-time.Hour),
	)
	video := createAdminVideoIntegrationVideo(
		t,
		db,
		owner.ID,
		"hide-invalid",
		"private video",
		entity.VideoProcessingReady,
		entity.VideoPublishPrivate,
		now.Add(-30*time.Minute),
		nil,
	)

	result, err := repository.Hide(
		context.Background(),
		admin.ID,
		video.ID,
		"規約違反",
		"request-hide-invalid",
		now,
	)
	if !errors.Is(err, entity.ErrVideoStateConflict) {
		t.Fatalf("error = %v, want ErrVideoStateConflict", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}

	var auditCount int64
	if err := db.Model(&entity.AdminAuditLog{}).
		Where("request_id = ?", "request-hide-invalid").
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("audit count = %d, want 0", auditCount)
	}
}

func TestAdminAuditLogVideoActionConstraintRejectsInvalidTransition(t *testing.T) {
	db := openAdminVideoIntegrationDB(t)
	now := time.Date(2026, 8, 6, 6, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	admin := createAdminVideoIntegrationUser(
		t,
		db,
		"Admin",
		"admin-audit-constraint@example.com",
		entity.RoleAdmin,
		entity.StatusActive,
		now.Add(-time.Hour),
	)

	invalid := &entity.AdminAuditLog{
		AdminUserID:  admin.ID,
		TargetType:   entity.AdminAuditTargetVideo,
		TargetID:     10,
		Action:       entity.AdminAuditActionVideoHide,
		BeforeStatus: string(entity.VideoPublishPrivate),
		AfterStatus:  string(entity.VideoPublishHidden),
		Reason:       "invalid transition",
		RequestID:    "request-invalid-audit-transition",
		CreatedAt:    now,
	}
	if err := db.Create(invalid).Error; err == nil {
		t.Fatal("invalid audit transition was accepted")
	}
}
