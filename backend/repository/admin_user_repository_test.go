//go:build integration

package repository

import (
	"coffee-reel/entity"
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdminUserRepositoryListUsersUsesCursorAndExcludesAdmins(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewAdminUserRepository(db)
	base := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)

	users := []*entity.User{
		{Name: "admin", Email: "admin-list@example.com", PasswordHash: "hash", Role: entity.RoleAdmin, Status: entity.StatusActive, CreatedAt: base.Add(4 * time.Minute), UpdatedAt: base.Add(4 * time.Minute)},
		{Name: "user-3", Email: "user-3@example.com", PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusActive, CreatedAt: base.Add(3 * time.Minute), UpdatedAt: base.Add(3 * time.Minute)},
		{Name: "user-2", Email: "user-2@example.com", PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusSuspended, CreatedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(2 * time.Minute)},
		{Name: "user-1", Email: "user-1@example.com", PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusActive, CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)},
	}
	for _, user := range users {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	first, err := repository.ListUsers(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("ListUsers(first) error = %v", err)
	}
	if len(first.Users) != 2 || !first.HasMore {
		t.Fatalf("first = %#v", first)
	}
	if first.Users[0].Name != "user-3" || first.Users[1].Name != "user-2" {
		t.Fatalf("first users = %#v", first.Users)
	}

	second, err := repository.ListUsers(context.Background(), 2, &AdminUserCursor{CreatedAt: first.LastCreatedAt, ID: first.LastID})
	if err != nil {
		t.Fatalf("ListUsers(second) error = %v", err)
	}
	if len(second.Users) != 1 || second.HasMore || second.Users[0].Name != "user-1" {
		t.Fatalf("second = %#v", second)
	}
}

func TestAdminUserRepositorySuspendAndResumeAreAtomic(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewAdminUserRepository(db)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	admin := &entity.User{Name: "admin", Email: "admin-state@example.com", PasswordHash: "hash", Role: entity.RoleAdmin, Status: entity.StatusActive, CreatedAt: now, UpdatedAt: now}
	user := &entity.User{Name: "user", Email: "user-state@example.com", PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 3, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}

	token := &entity.RefreshToken{
		UserID:    user.ID,
		TokenHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FamilyID:  "family-state",
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	suspended, err := repository.SuspendUser(context.Background(), admin.ID, user.ID, "policy violation", "request-suspend", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("SuspendUser() error = %v", err)
	}
	if suspended.Status != entity.StatusSuspended || suspended.TokenVersion != 4 {
		t.Fatalf("suspended user = %#v", suspended)
	}

	var storedToken entity.RefreshToken
	if err := db.First(&storedToken, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedToken.RevokedAt == nil {
		t.Fatal("refresh token was not revoked")
	}

	var auditCount int64
	if err := db.Model(&entity.AdminAuditLog{}).Where("target_id = ?", user.ID).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("auditCount = %d, want 1", auditCount)
	}

	_, err = repository.SuspendUser(context.Background(), admin.ID, user.ID, "duplicate", "request-duplicate", now.Add(2*time.Minute))
	if !errors.Is(err, entity.ErrUserStatusConflict) {
		t.Fatalf("second SuspendUser() error = %v", err)
	}
	if err := db.Model(&entity.AdminAuditLog{}).Where("target_id = ?", user.ID).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("duplicate auditCount = %d, want 1", auditCount)
	}

	resumed, err := repository.ResumeUser(context.Background(), admin.ID, user.ID, "review complete", "request-resume", now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("ResumeUser() error = %v", err)
	}
	if resumed.Status != entity.StatusActive || resumed.TokenVersion != 4 {
		t.Fatalf("resumed user = %#v", resumed)
	}

	if err := db.First(&storedToken, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedToken.RevokedAt == nil {
		t.Fatal("revoked token was restored")
	}
	if err := db.Model(&entity.AdminAuditLog{}).Where("target_id = ?", user.ID).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("auditCount = %d, want 2", auditCount)
	}
}

func TestAdminUserRepositoryRollsBackWhenAuditLogIsInvalid(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewAdminUserRepository(db)
	now := time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)

	admin := &entity.User{Name: "admin", Email: "admin-rollback@example.com", PasswordHash: "hash", Role: entity.RoleAdmin, Status: entity.StatusActive, CreatedAt: now, UpdatedAt: now}
	user := &entity.User{Name: "user", Email: "user-rollback@example.com", PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 5, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}

	token := &entity.RefreshToken{
		UserID:    user.ID,
		TokenHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		FamilyID:  "family-rollback",
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	_, err := repository.SuspendUser(
		context.Background(),
		admin.ID,
		user.ID,
		"   ",
		"request-rollback",
		now.Add(time.Minute),
	)
	if !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("SuspendUser() error = %v, want ErrInvalidInput", err)
	}

	var storedUser entity.User
	if err := db.First(&storedUser, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedUser.Status != entity.StatusActive || storedUser.TokenVersion != 5 {
		t.Fatalf("storedUser = %#v, want unchanged active user", storedUser)
	}

	var storedToken entity.RefreshToken
	if err := db.First(&storedToken, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedToken.RevokedAt != nil {
		t.Fatalf("RevokedAt = %v, want nil after rollback", storedToken.RevokedAt)
	}

	var auditCount int64
	if err := db.Model(&entity.AdminAuditLog{}).Where("target_id = ?", user.ID).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 {
		t.Fatalf("auditCount = %d, want 0", auditCount)
	}
}
