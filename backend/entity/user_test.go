package entity

import (
	"encoding/json"
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
