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
	before := time.Now()
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

func TestAdminUserUsecaseCreateAdminHandlesUniqueRace(t *testing.T) {
	tests := []struct {
		name         string
		existing     *entity.User
		wantErr      error
		wantExisting bool
	}{
		{
			name: "concurrent admin creation is idempotent",
			existing: &entity.User{
				ID:     20,
				Name:   "Existing Admin",
				Email:  "admin@example.com",
				Role:   entity.RoleAdmin,
				Status: entity.StatusActive,
			},
			wantExisting: true,
		},
		{
			name: "concurrent general user creation is not promoted",
			existing: &entity.User{
				ID:     21,
				Name:   "Existing User",
				Email:  "admin@example.com",
				Role:   entity.RoleUser,
				Status: entity.StatusActive,
			},
			wantErr: entity.ErrEmailAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findCalls := 0
			createCalls := 0
			hashCalls := 0

			users := &userRepositoryMock{
				findByEmailFunc: func(_ context.Context, email string) (*entity.User, error) {
					if email != "admin@example.com" {
						t.Fatalf("email = %q", email)
					}

					findCalls++
					if findCalls == 1 {
						return nil, entity.ErrUserNotFound
					}
					return tt.existing, nil
				},
				createFunc: func(_ context.Context, user *entity.User) error {
					createCalls++
					if user.Role != entity.RoleAdmin || user.Status != entity.StatusActive {
						t.Fatalf("admin = %#v", user)
					}
					return entity.ErrEmailAlreadyExists
				},
			}

			tokens := &tokenServiceMock{
				hashPasswordFunc: func(password string) (string, error) {
					hashCalls++
					if password != "password123" {
						t.Fatalf("password = %q", password)
					}
					return "hashed-password", nil
				},
			}

			adminUsecase := NewAdminUserUsecase(
				users,
				&adminUserRepositoryMock{},
				tokens,
			)

			admin, created, err := adminUsecase.CreateAdmin(
				context.Background(),
				"Admin",
				"admin@example.com",
				"password123",
			)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateAdmin() error = %v, want %v", err, tt.wantErr)
			}
			if created {
				t.Fatal("created = true, want false after unique conflict")
			}

			if tt.wantExisting {
				if admin != tt.existing {
					t.Fatalf(
						"admin = %#v, want existing admin %#v",
						admin,
						tt.existing,
					)
				}
			} else if admin != nil {
				t.Fatalf("admin = %#v, want nil", admin)
			}

			if findCalls != 2 {
				t.Fatalf("FindByEmail calls = %d, want 2", findCalls)
			}
			if createCalls != 1 {
				t.Fatalf("Create calls = %d, want 1", createCalls)
			}
			if hashCalls != 1 {
				t.Fatalf("HashPassword calls = %d, want 1", hashCalls)
			}
		})
	}
}

func TestAdminUserUsecaseListUsersReturnsFinalPageWithoutNextCursor(t *testing.T) {
	inputCursorTime := time.Date(
		2026,
		7,
		23,
		19,
		0,
		0,
		0,
		time.FixedZone("JST", 9*60*60),
	)
	itemCreatedAt := time.Date(
		2026,
		7,
		22,
		18,
		30,
		0,
		456,
		time.FixedZone("JST", 9*60*60),
	)

	repositoryMock := &adminUserRepositoryMock{
		listUsersFunc: func(
			_ context.Context,
			limit int,
			cursor *repository.AdminUserCursor,
		) (repository.AdminUserListResult, error) {
			if limit != 10 {
				t.Fatalf("limit = %d, want 10", limit)
			}
			if cursor == nil {
				t.Fatal("cursor is nil")
			}
			if cursor.ID != 7 || !cursor.CreatedAt.Equal(inputCursorTime.UTC()) {
				t.Fatalf("cursor = %#v", cursor)
			}
			if cursor.CreatedAt.Location() != time.UTC {
				t.Fatalf(
					"cursor location = %s, want UTC",
					cursor.CreatedAt.Location(),
				)
			}

			return repository.AdminUserListResult{
				Users: []repository.AdminUserListItem{
					{
						ID:        6,
						Name:      "Final User",
						Email:     "final@example.com",
						Status:    entity.StatusActive,
						CreatedAt: itemCreatedAt,
					},
				},
				HasMore:       false,
				LastCreatedAt: itemCreatedAt,
				LastID:        6,
			}, nil
		},
	}

	adminUsecase := NewAdminUserUsecase(
		&userRepositoryMock{},
		repositoryMock,
		&tokenServiceMock{},
	)

	result, err := adminUsecase.ListUsers(
		context.Background(),
		&entity.User{
			ID:     99,
			Role:   entity.RoleAdmin,
			Status: entity.StatusActive,
		},
		AdminUserListInput{
			Limit: 10,
			Cursor: &AdminUserCursor{
				CreatedAt: inputCursorTime,
				ID:        7,
			},
		},
	)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if result.HasMore {
		t.Fatal("HasMore = true, want false")
	}
	if result.NextCursor != nil {
		t.Fatalf("NextCursor = %q, want nil", *result.NextCursor)
	}
	if len(result.Items) != 1 {
		t.Fatalf("Items length = %d, want 1", len(result.Items))
	}
	if !result.Items[0].CreatedAt.Equal(itemCreatedAt.UTC()) {
		t.Fatalf(
			"CreatedAt = %s, want %s",
			result.Items[0].CreatedAt,
			itemCreatedAt.UTC(),
		)
	}
	if result.Items[0].CreatedAt.Location() != time.UTC {
		t.Fatalf(
			"CreatedAt location = %s, want UTC",
			result.Items[0].CreatedAt.Location(),
		)
	}
}

func TestAdminUserUsecaseListUsersReturnsEmptySliceWithoutCursor(t *testing.T) {
	repositoryMock := &adminUserRepositoryMock{
		listUsersFunc: func(
			_ context.Context,
			limit int,
			cursor *repository.AdminUserCursor,
		) (repository.AdminUserListResult, error) {
			if limit != 20 || cursor != nil {
				t.Fatalf("limit=%d cursor=%#v", limit, cursor)
			}
			return repository.AdminUserListResult{}, nil
		},
	}

	adminUsecase := NewAdminUserUsecase(
		&userRepositoryMock{},
		repositoryMock,
		&tokenServiceMock{},
	)

	result, err := adminUsecase.ListUsers(
		context.Background(),
		&entity.User{
			ID:     99,
			Role:   entity.RoleAdmin,
			Status: entity.StatusActive,
		},
		AdminUserListInput{Limit: 20},
	)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if result.Items == nil {
		t.Fatal("Items is nil, want empty slice")
	}
	if len(result.Items) != 0 {
		t.Fatalf("Items length = %d, want 0", len(result.Items))
	}
	if result.HasMore {
		t.Fatal("HasMore = true, want false")
	}
	if result.NextCursor != nil {
		t.Fatalf("NextCursor = %q, want nil", *result.NextCursor)
	}
}

func TestAdminUserUsecaseGetUserDetail(t *testing.T) {
	userCreatedAt := time.Date(
		2026,
		7,
		20,
		18,
		0,
		0,
		123,
		time.FixedZone("JST", 9*60*60),
	)
	videoCreatedAt := time.Date(
		2026,
		7,
		21,
		20,
		30,
		0,
		456,
		time.FixedZone("JST", 9*60*60),
	)

	repositoryMock := &adminUserRepositoryMock{
		findDetailFunc: func(
			_ context.Context,
			userID uint64,
		) (*repository.AdminUserDetail, error) {
			if userID != 3 {
				t.Fatalf("userID = %d, want 3", userID)
			}

			return &repository.AdminUserDetail{
				ID:        3,
				Name:      "User",
				Email:     "user@example.com",
				Status:    entity.StatusSuspended,
				CreatedAt: userCreatedAt,
				Videos: []repository.AdminUserVideoItem{
					{
						ID:               30,
						Title:            "Coffee Video",
						ProcessingStatus: "ready",
						PublishStatus:    "hidden",
						CreatedAt:        videoCreatedAt,
					},
				},
			}, nil
		},
	}

	adminUsecase := NewAdminUserUsecase(
		&userRepositoryMock{},
		repositoryMock,
		&tokenServiceMock{},
	)

	result, err := adminUsecase.GetUserDetail(
		context.Background(),
		&entity.User{
			ID:     99,
			Role:   entity.RoleAdmin,
			Status: entity.StatusActive,
		},
		3,
	)
	if err != nil {
		t.Fatalf("GetUserDetail() error = %v", err)
	}

	if result.ID != 3 ||
		result.Name != "User" ||
		result.Email != "user@example.com" ||
		result.Status != entity.StatusSuspended {
		t.Fatalf("result = %#v", result)
	}

	if !result.CreatedAt.Equal(userCreatedAt.UTC()) ||
		result.CreatedAt.Location() != time.UTC {
		t.Fatalf(
			"CreatedAt = %s, want UTC %s",
			result.CreatedAt,
			userCreatedAt.UTC(),
		)
	}

	if len(result.Videos) != 1 {
		t.Fatalf("Videos length = %d, want 1", len(result.Videos))
	}

	video := result.Videos[0]
	if video.ID != 30 ||
		video.Title != "Coffee Video" ||
		video.ProcessingStatus != "ready" ||
		video.PublishStatus != "hidden" {
		t.Fatalf("video = %#v", video)
	}

	if !video.CreatedAt.Equal(videoCreatedAt.UTC()) ||
		video.CreatedAt.Location() != time.UTC {
		t.Fatalf(
			"video CreatedAt = %s, want UTC %s",
			video.CreatedAt,
			videoCreatedAt.UTC(),
		)
	}
}

func TestAdminUserUsecaseResumeUser(t *testing.T) {
	before := time.Now()

	repositoryMock := &adminUserRepositoryMock{
		resumeUserFunc: func(
			_ context.Context,
			adminID uint64,
			targetID uint64,
			reason string,
			requestID string,
			now time.Time,
		) (*entity.User, error) {
			if adminID != 9 ||
				targetID != 3 ||
				reason != "review complete" ||
				requestID != "request-2" {
				t.Fatalf(
					"inputs = %d %d %q %q",
					adminID,
					targetID,
					reason,
					requestID,
				)
			}
			if now.Before(before) || now.Location() != time.UTC {
				t.Fatalf("now = %s", now)
			}

			return &entity.User{
				ID:     3,
				Status: entity.StatusActive,
				UpdatedAt: now.In(
					time.FixedZone("JST", 9*60*60),
				),
			}, nil
		},
	}

	adminUsecase := NewAdminUserUsecase(
		&userRepositoryMock{},
		repositoryMock,
		&tokenServiceMock{},
	)

	result, err := adminUsecase.ResumeUser(
		context.Background(),
		&entity.User{
			ID:     9,
			Role:   entity.RoleAdmin,
			Status: entity.StatusActive,
		},
		3,
		"review complete",
		"request-2",
	)
	if err != nil {
		t.Fatalf("ResumeUser() error = %v", err)
	}
	if result.ID != 3 || result.Status != entity.StatusActive {
		t.Fatalf("result = %#v", result)
	}
	if result.UpdatedAt.Before(before) ||
		result.UpdatedAt.Location() != time.UTC {
		t.Fatalf("UpdatedAt = %s", result.UpdatedAt)
	}
}

func TestAdminUserUsecaseRejectsMissingOrInactiveAdmin(t *testing.T) {
	tests := []struct {
		name  string
		actor *entity.User
	}{
		{
			name:  "missing actor",
			actor: nil,
		},
		{
			name: "suspended admin",
			actor: &entity.User{
				ID:     9,
				Role:   entity.RoleAdmin,
				Status: entity.StatusSuspended,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adminUsecase := NewAdminUserUsecase(
				&userRepositoryMock{},
				&adminUserRepositoryMock{},
				&tokenServiceMock{},
			)

			_, err := adminUsecase.ListUsers(
				context.Background(),
				tt.actor,
				AdminUserListInput{Limit: 20},
			)
			if !errors.Is(err, entity.ErrUnauthorized) {
				t.Fatalf(
					"ListUsers() error = %v, want ErrUnauthorized",
					err,
				)
			}
		})
	}
}

func TestAdminUserUsecasePropagatesRepositoryErrors(t *testing.T) {
	repositoryErr := errors.New("repository failure")
	actor := &entity.User{
		ID:     9,
		Role:   entity.RoleAdmin,
		Status: entity.StatusActive,
	}

	tests := []struct {
		name string
		call func(IAdminUserUsecase) error
		repo *adminUserRepositoryMock
	}{
		{
			name: "list users",
			repo: &adminUserRepositoryMock{
				listUsersFunc: func(
					context.Context,
					int,
					*repository.AdminUserCursor,
				) (repository.AdminUserListResult, error) {
					return repository.AdminUserListResult{}, repositoryErr
				},
			},
			call: func(adminUsecase IAdminUserUsecase) error {
				_, err := adminUsecase.ListUsers(
					context.Background(),
					actor,
					AdminUserListInput{Limit: 20},
				)
				return err
			},
		},
		{
			name: "get user detail",
			repo: &adminUserRepositoryMock{
				findDetailFunc: func(
					context.Context,
					uint64,
				) (*repository.AdminUserDetail, error) {
					return nil, repositoryErr
				},
			},
			call: func(adminUsecase IAdminUserUsecase) error {
				_, err := adminUsecase.GetUserDetail(
					context.Background(),
					actor,
					3,
				)
				return err
			},
		},
		{
			name: "suspend user",
			repo: &adminUserRepositoryMock{
				suspendUserFunc: func(
					context.Context,
					uint64,
					uint64,
					string,
					string,
					time.Time,
				) (*entity.User, error) {
					return nil, repositoryErr
				},
			},
			call: func(adminUsecase IAdminUserUsecase) error {
				_, err := adminUsecase.SuspendUser(
					context.Background(),
					actor,
					3,
					"reason",
					"request-1",
				)
				return err
			},
		},
		{
			name: "resume user",
			repo: &adminUserRepositoryMock{
				resumeUserFunc: func(
					context.Context,
					uint64,
					uint64,
					string,
					string,
					time.Time,
				) (*entity.User, error) {
					return nil, repositoryErr
				},
			},
			call: func(adminUsecase IAdminUserUsecase) error {
				_, err := adminUsecase.ResumeUser(
					context.Background(),
					actor,
					3,
					"reason",
					"request-2",
				)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adminUsecase := NewAdminUserUsecase(
				&userRepositoryMock{},
				tt.repo,
				&tokenServiceMock{},
			)

			if err := tt.call(adminUsecase); !errors.Is(err, repositoryErr) {
				t.Fatalf("error = %v, want repository error", err)
			}
		})
	}
}
