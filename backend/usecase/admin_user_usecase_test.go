package usecase

import (
	"coffee-reel/entity"
	"coffee-reel/repository"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type adminUserRepositoryMock struct {
	listUsersFunc   func(context.Context, int, *repository.AdminUserCursor) (repository.AdminUserListResult, error)
	findDetailFunc  func(context.Context, uint64) (*repository.AdminUserDetail, error)
	suspendUserFunc func(context.Context, uint64, uint64, string, string, time.Time) (*entity.User, error)
	resumeUserFunc  func(context.Context, uint64, uint64, string, string, time.Time) (*entity.User, error)
}

func (m *adminUserRepositoryMock) ListUsers(ctx context.Context, limit int, cursor *repository.AdminUserCursor) (repository.AdminUserListResult, error) {
	if m.listUsersFunc == nil {
		panic("unexpected AdminUserRepository.ListUsers call")
	}
	return m.listUsersFunc(ctx, limit, cursor)
}

func (m *adminUserRepositoryMock) FindUserDetail(ctx context.Context, userID uint64) (*repository.AdminUserDetail, error) {
	if m.findDetailFunc == nil {
		panic("unexpected AdminUserRepository.FindUserDetail call")
	}
	return m.findDetailFunc(ctx, userID)
}

func (m *adminUserRepositoryMock) SuspendUser(ctx context.Context, adminUserID, targetUserID uint64, reason, requestID string, now time.Time) (*entity.User, error) {
	if m.suspendUserFunc == nil {
		panic("unexpected AdminUserRepository.SuspendUser call")
	}
	return m.suspendUserFunc(ctx, adminUserID, targetUserID, reason, requestID, now)
}

func (m *adminUserRepositoryMock) ResumeUser(ctx context.Context, adminUserID, targetUserID uint64, reason, requestID string, now time.Time) (*entity.User, error) {
	if m.resumeUserFunc == nil {
		panic("unexpected AdminUserRepository.ResumeUser call")
	}
	return m.resumeUserFunc(ctx, adminUserID, targetUserID, reason, requestID, now)
}

func TestAdminUserUsecaseCreateAdmin(t *testing.T) {
	users := &userRepositoryMock{
		findByEmailFunc: func(context.Context, string) (*entity.User, error) {
			return nil, entity.ErrUserNotFound
		},
		createFunc: func(_ context.Context, user *entity.User) error {
			if user.Role != entity.RoleAdmin || user.Status != entity.StatusActive || user.TokenVersion != 0 {
				t.Fatalf("admin = %#v", user)
			}
			if user.PasswordHash != "hashed-password" {
				t.Fatalf("PasswordHash = %q", user.PasswordHash)
			}
			user.ID = 10
			return nil
		},
	}
	tokens := &tokenServiceMock{hashPasswordFunc: func(password string) (string, error) {
		if password != "password123" {
			t.Fatalf("password = %q", password)
		}
		return "hashed-password", nil
	}}

	usecase := NewAdminUserUsecase(users, &adminUserRepositoryMock{}, tokens)
	admin, created, err := usecase.CreateAdmin(context.Background(), "Admin", "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("CreateAdmin() error = %v", err)
	}
	if !created || admin.ID != 10 || admin.Role != entity.RoleAdmin {
		t.Fatalf("CreateAdmin() = %#v, created=%v", admin, created)
	}
	if admin.PasswordHash == "password123" {
		t.Fatal("plain password was stored")
	}
}

func TestAdminUserUsecaseCreateAdminIsIdempotentForExistingAdmin(t *testing.T) {
	existing := &entity.User{ID: 5, Role: entity.RoleAdmin, Status: entity.StatusActive}
	users := &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
		return existing, nil
	}}

	usecase := NewAdminUserUsecase(users, &adminUserRepositoryMock{}, &tokenServiceMock{})
	admin, created, err := usecase.CreateAdmin(context.Background(), "Admin", "admin@example.com", "password123")
	if err != nil || created || admin != existing {
		t.Fatalf("CreateAdmin() = %#v, %v, %v", admin, created, err)
	}
}

func TestAdminUserUsecaseCreateAdminDoesNotPromoteUser(t *testing.T) {
	users := &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
		return &entity.User{ID: 5, Role: entity.RoleUser, Status: entity.StatusActive}, nil
	}}

	usecase := NewAdminUserUsecase(users, &adminUserRepositoryMock{}, &tokenServiceMock{})
	_, _, err := usecase.CreateAdmin(context.Background(), "Admin", "admin@example.com", "password123")
	if !errors.Is(err, entity.ErrEmailAlreadyExists) {
		t.Fatalf("CreateAdmin() error = %v", err)
	}
}

func TestAdminUserUsecaseListUsersGeneratesCursorFromLastReturnedUser(t *testing.T) {
	lastTime := time.Date(2026, 7, 23, 10, 0, 0, 123, time.FixedZone("JST", 9*60*60))
	repositoryMock := &adminUserRepositoryMock{listUsersFunc: func(_ context.Context, limit int, cursor *repository.AdminUserCursor) (repository.AdminUserListResult, error) {
		if limit != 20 || cursor != nil {
			t.Fatalf("limit=%d cursor=%#v", limit, cursor)
		}
		return repository.AdminUserListResult{
			Users: []repository.AdminUserListItem{
				{ID: 2, Name: "A", Email: "a@example.com", Status: entity.StatusActive, CreatedAt: lastTime.Add(time.Hour)},
				{ID: 1, Name: "B", Email: "b@example.com", Status: entity.StatusSuspended, CreatedAt: lastTime},
			},
			HasMore:       true,
			LastCreatedAt: lastTime,
			LastID:        1,
		}, nil
	}}

	actor := &entity.User{ID: 99, Role: entity.RoleAdmin, Status: entity.StatusActive}
	usecase := NewAdminUserUsecase(&userRepositoryMock{}, repositoryMock, &tokenServiceMock{})
	result, err := usecase.ListUsers(context.Background(), actor, AdminUserListInput{Limit: 20})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if !result.HasMore || result.NextCursor == nil || len(result.Items) != 2 {
		t.Fatalf("result = %#v", result)
	}

	payload, err := base64.RawURLEncoding.DecodeString(*result.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	var cursor AdminUserCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		t.Fatal(err)
	}
	if cursor.ID != 1 || !cursor.CreatedAt.Equal(lastTime.UTC()) {
		t.Fatalf("cursor = %#v", cursor)
	}
}

func TestAdminUserUsecaseRejectsNonAdminActor(t *testing.T) {
	usecase := NewAdminUserUsecase(&userRepositoryMock{}, &adminUserRepositoryMock{}, &tokenServiceMock{})
	_, err := usecase.ListUsers(
		context.Background(),
		&entity.User{ID: 1, Role: entity.RoleUser, Status: entity.StatusActive},
		AdminUserListInput{Limit: 20},
	)
	if !errors.Is(err, entity.ErrAdminRequired) {
		t.Fatalf("ListUsers() error = %v", err)
	}
}

func TestAdminUserUsecaseSuspendUser(t *testing.T) {
	before := time.Now().UTC()
	repositoryMock := &adminUserRepositoryMock{suspendUserFunc: func(_ context.Context, adminID, targetID uint64, reason, requestID string, now time.Time) (*entity.User, error) {
		if adminID != 9 || targetID != 3 || reason != "reason" || requestID != "request-1" {
			t.Fatalf("inputs = %d %d %q %q", adminID, targetID, reason, requestID)
		}
		if now.Before(before) || now.Location() != time.UTC {
			t.Fatalf("now = %s", now)
		}
		return &entity.User{ID: 3, Status: entity.StatusSuspended, UpdatedAt: now}, nil
	}}

	usecase := NewAdminUserUsecase(&userRepositoryMock{}, repositoryMock, &tokenServiceMock{})
	result, err := usecase.SuspendUser(
		context.Background(),
		&entity.User{ID: 9, Role: entity.RoleAdmin, Status: entity.StatusActive},
		3,
		"reason",
		"request-1",
	)
	if err != nil || result.Status != entity.StatusSuspended {
		t.Fatalf("SuspendUser() = %#v, %v", result, err)
	}
}

func TestAdminUserUsecaseRejectsSelfManagement(t *testing.T) {
	usecase := NewAdminUserUsecase(&userRepositoryMock{}, &adminUserRepositoryMock{}, &tokenServiceMock{})
	_, err := usecase.SuspendUser(
		context.Background(),
		&entity.User{ID: 9, Role: entity.RoleAdmin, Status: entity.StatusActive},
		9,
		"reason",
		"request-1",
	)
	if !errors.Is(err, entity.ErrUserManagementForbidden) {
		t.Fatalf("SuspendUser() error = %v", err)
	}
}
