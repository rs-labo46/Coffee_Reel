package entity

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRefreshTokenExpirationBoundary(t *testing.T) {
	expiresAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	token := RefreshToken{ExpiresAt: expiresAt}

	if token.IsExpired(expiresAt.Add(-time.Nanosecond)) {
		t.Fatal("token must be usable immediately before ExpiresAt")
	}
	if !token.IsExpired(expiresAt) {
		t.Fatal("token must be expired exactly at ExpiresAt")
	}
	if !token.IsExpired(expiresAt.Add(time.Second)) {
		t.Fatal("token must be expired after ExpiresAt")
	}

	jstAtSameInstant := expiresAt.In(time.FixedZone("JST", 9*60*60))
	if !token.IsExpired(jstAtSameInstant) {
		t.Fatal("expiration comparison must use the instant, not the location")
	}
}

func TestRefreshTokenIsUsable(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	past := now.Add(-time.Minute)

	tests := []struct {
		name  string
		token RefreshToken
		want  bool
	}{
		{name: "unused and active", token: RefreshToken{ExpiresAt: now.Add(time.Hour)}, want: true},
		{name: "expired", token: RefreshToken{ExpiresAt: now}, want: false},
		{name: "used", token: RefreshToken{ExpiresAt: now.Add(time.Hour), UsedAt: &past}, want: false},
		{name: "revoked", token: RefreshToken{ExpiresAt: now.Add(time.Hour), RevokedAt: &past}, want: false},
		{name: "used and revoked", token: RefreshToken{ExpiresAt: now.Add(time.Hour), UsedAt: &past, RevokedAt: &past}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.IsUsable(now); got != tt.want {
				t.Fatalf("IsUsable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefreshTokenMarkUsed(t *testing.T) {
	now := time.Date(2026, 7, 21, 21, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	token := RefreshToken{}

	token.MarkUsed(99, now)

	if token.UsedAt == nil || !token.UsedAt.Equal(now) {
		t.Fatalf("UsedAt = %v, want %s", token.UsedAt, now)
	}
	if token.ReplacedByID == nil || *token.ReplacedByID != 99 {
		t.Fatalf("ReplacedByID = %v, want 99", token.ReplacedByID)
	}
}

func TestRefreshTokenRevokeIsIdempotent(t *testing.T) {
	first := time.Date(2026, 7, 21, 1, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	second := first.Add(time.Hour)
	token := RefreshToken{}

	token.Revoke(first)
	if token.RevokedAt == nil || !token.RevokedAt.Equal(first) {
		t.Fatalf("first Revoke() set RevokedAt = %v, want %s", token.RevokedAt, first)
	}

	token.Revoke(second)
	if token.RevokedAt == nil || !token.RevokedAt.Equal(first) {
		t.Fatalf("second Revoke() changed RevokedAt = %v, want original %s", token.RevokedAt, first)
	}
}

func TestRefreshTokenJSONExposesNothing(t *testing.T) {
	now := time.Now()
	replacedByID := uint64(2)
	token := RefreshToken{
		ID:           1,
		UserID:       10,
		TokenHash:    "hash",
		FamilyID:     "family",
		ReplacedByID: &replacedByID,
		ExpiresAt:    now.Add(time.Hour),
		UsedAt:       &now,
		RevokedAt:    &now,
		CreatedAt:    now,
	}

	body, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(body) != "{}" {
		t.Fatalf("RefreshToken JSON = %s, want {}", body)
	}
}
