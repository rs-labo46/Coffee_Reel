package entity

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUserRoleAndStatusPredicates(t *testing.T) {
	tests := []struct {
		name       string
		user       User
		wantActive bool
		wantAdmin  bool
	}{
		{name: "active user", user: User{Role: RoleUser, Status: StatusActive}, wantActive: true, wantAdmin: false},
		{name: "suspended user", user: User{Role: RoleUser, Status: StatusSuspended}, wantActive: false, wantAdmin: false},
		{name: "active admin", user: User{Role: RoleAdmin, Status: StatusActive}, wantActive: true, wantAdmin: true},
		{name: "unknown values are never privileged", user: User{Role: UserRole("owner"), Status: UserStatus("enabled")}, wantActive: false, wantAdmin: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.IsActive(); got != tt.wantActive {
				t.Fatalf("IsActive() = %v, want %v", got, tt.wantActive)
			}
			if got := tt.user.IsAdmin(); got != tt.wantAdmin {
				t.Fatalf("IsAdmin() = %v, want %v", got, tt.wantAdmin)
			}
		})
	}
}

func TestUserInvalidateAccessTokens(t *testing.T) {
	originalCreatedAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	now := time.Date(2026, 7, 21, 8, 30, 0, 123, time.FixedZone("JST", 9*60*60))
	user := User{
		ID:           10,
		Name:         "user",
		Email:        "user@example.com",
		PasswordHash: "secret-hash",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 7,
		CreatedAt:    originalCreatedAt,
		UpdatedAt:    originalCreatedAt,
	}

	user.InvalidateAccessTokens(now)

	if user.TokenVersion != 8 {
		t.Fatalf("TokenVersion = %d, want 8", user.TokenVersion)
	}
	if !user.UpdatedAt.Equal(now.UTC()) {
		t.Fatalf("UpdatedAt = %s, want %s", user.UpdatedAt, now.UTC())
	}
	if user.UpdatedAt.Location() != time.UTC {
		t.Fatalf("UpdatedAt location = %s, want UTC", user.UpdatedAt.Location())
	}
	if !user.CreatedAt.Equal(originalCreatedAt) {
		t.Fatal("InvalidateAccessTokens must not change CreatedAt")
	}
	if user.Status != StatusActive || user.Role != RoleUser {
		t.Fatal("InvalidateAccessTokens must not change role or status")
	}
}

func TestUserJSONDoesNotExposeAuthenticationSecrets(t *testing.T) {
	user := User{
		ID:           1,
		Name:         "Alice",
		Email:        "alice@example.com",
		PasswordHash: "$2a$10$do-not-expose",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 42,
		CreatedAt:    time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC),
	}

	body, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonText := string(body)
	for _, secret := range []string{"password_hash", "PasswordHash", user.PasswordHash, "token_version", "TokenVersion", "42"} {
		if strings.Contains(jsonText, secret) {
			t.Fatalf("JSON contains secret %q: %s", secret, jsonText)
		}
	}
	for _, publicField := range []string{"\"id\"", "\"name\"", "\"email\"", "\"role\"", "\"status\""} {
		if !strings.Contains(jsonText, publicField) {
			t.Fatalf("JSON is missing public field %s: %s", publicField, jsonText)
		}
	}
}

//----追加コード----

func TestUserSuspend(t *testing.T) {
	createdAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	now := time.Date(2026, 7, 21, 8, 30, 0, 123, time.FixedZone("JST", 9*60*60))

	user := User{
		ID:           10,
		Name:         "user",
		Email:        "user@example.com",
		PasswordHash: "secret-hash",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 7,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}

	err := user.Suspend(now)
	if err != nil {
		t.Fatalf("Suspend() error = %v", err)
	}

	if user.Status != StatusSuspended {
		t.Fatalf("Status = %q, want %q", user.Status, StatusSuspended)
	}
	if user.TokenVersion != 8 {
		t.Fatalf("TokenVersion = %d, want 8", user.TokenVersion)
	}
	if !user.UpdatedAt.Equal(now.UTC()) {
		t.Fatalf("UpdatedAt = %s, want %s", user.UpdatedAt, now.UTC())
	}
	if user.UpdatedAt.Location() != time.UTC {
		t.Fatalf("UpdatedAt location = %s, want UTC", user.UpdatedAt.Location())
	}
	if !user.CreatedAt.Equal(createdAt) {
		t.Fatal("Suspend must not change CreatedAt")
	}
	if user.Role != RoleUser {
		t.Fatalf("Role = %q, want %q", user.Role, RoleUser)
	}
	if user.ID != 10 ||
		user.Name != "user" ||
		user.Email != "user@example.com" ||
		user.PasswordHash != "secret-hash" {
		t.Fatalf("Suspend changed immutable user fields: %+v", user)
	}
}

//------------

func TestUserSuspendRejectsInvalidState(t *testing.T) {
	now := time.Date(2026, 7, 21, 8, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		user    User
		wantErr error
	}{
		{
			name: "admin cannot be suspended",
			user: User{
				Role:         RoleAdmin,
				Status:       StatusActive,
				TokenVersion: 3,
			},
			wantErr: ErrUserManagementForbidden,
		},
		{
			name: "unknown role cannot be suspended",
			user: User{
				Role:         UserRole("owner"),
				Status:       StatusActive,
				TokenVersion: 3,
			},
			wantErr: ErrUserManagementForbidden,
		},
		{
			name: "already suspended user is rejected",
			user: User{
				Role:         RoleUser,
				Status:       StatusSuspended,
				TokenVersion: 3,
			},
			wantErr: ErrUserStatusConflict,
		},
		{
			name: "unknown status is rejected",
			user: User{
				Role:         RoleUser,
				Status:       UserStatus("disabled"),
				TokenVersion: 3,
			},
			wantErr: ErrUserStatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.user

			err := tt.user.Suspend(now)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Suspend() error = %v, want %v", err, tt.wantErr)
			}
			if tt.user != before {
				t.Fatalf(
					"Suspend() changed user on error: got %+v, want %+v",
					tt.user,
					before,
				)
			}
		})
	}
}

func TestUserResume(t *testing.T) {
	createdAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	now := time.Date(2026, 7, 21, 8, 30, 0, 123, time.FixedZone("JST", 9*60*60))

	user := User{
		ID:           10,
		Name:         "user",
		Email:        "user@example.com",
		PasswordHash: "secret-hash",
		Role:         RoleUser,
		Status:       StatusSuspended,
		TokenVersion: 8,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}

	err := user.Resume(now)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if user.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", user.Status, StatusActive)
	}
	if user.TokenVersion != 8 {
		t.Fatalf("TokenVersion = %d, want 8", user.TokenVersion)
	}
	if !user.UpdatedAt.Equal(now.UTC()) {
		t.Fatalf("UpdatedAt = %s, want %s", user.UpdatedAt, now.UTC())
	}
	if user.UpdatedAt.Location() != time.UTC {
		t.Fatalf("UpdatedAt location = %s, want UTC", user.UpdatedAt.Location())
	}
	if !user.CreatedAt.Equal(createdAt) {
		t.Fatal("Resume must not change CreatedAt")
	}
	if user.Role != RoleUser {
		t.Fatalf("Role = %q, want %q", user.Role, RoleUser)
	}
}

func TestUserResumeRejectsInvalidState(t *testing.T) {
	now := time.Date(2026, 7, 21, 8, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		user    User
		wantErr error
	}{
		{
			name: "admin cannot be resumed",
			user: User{
				Role:         RoleAdmin,
				Status:       StatusSuspended,
				TokenVersion: 3,
			},
			wantErr: ErrUserManagementForbidden,
		},
		{
			name: "unknown role cannot be resumed",
			user: User{
				Role:         UserRole("owner"),
				Status:       StatusSuspended,
				TokenVersion: 3,
			},
			wantErr: ErrUserManagementForbidden,
		},
		{
			name: "already active user is rejected",
			user: User{
				Role:         RoleUser,
				Status:       StatusActive,
				TokenVersion: 3,
			},
			wantErr: ErrUserStatusConflict,
		},
		{
			name: "unknown status is rejected",
			user: User{
				Role:         RoleUser,
				Status:       UserStatus("disabled"),
				TokenVersion: 3,
			},
			wantErr: ErrUserStatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.user

			err := tt.user.Resume(now)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Resume() error = %v, want %v", err, tt.wantErr)
			}
			if tt.user != before {
				t.Fatalf(
					"Resume() changed user on error: got %+v, want %+v",
					tt.user,
					before,
				)
			}
		})
	}
}
