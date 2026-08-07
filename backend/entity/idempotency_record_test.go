package entity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewIdempotencyRecord(t *testing.T) {
	now := time.Date(2026, 7, 28, 3, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	expiresAt := now.Add(24 * time.Hour)
	keyHash := strings.Repeat("a", 64)
	requestHash := strings.Repeat("0", 64)

	record, err := NewIdempotencyRecord(10, keyHash, requestHash, expiresAt, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.Scope != IdempotencyScopeVideoCreate || record.ResourceID != 0 {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record.KeyHash != keyHash || record.RequestHash != requestHash {
		t.Fatalf("hashes changed unexpectedly: %#v", record)
	}
	if !record.CreatedAt.Equal(now) || !record.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("timestamps changed: %#v", record)
	}
}

func TestNewIdempotencyRecordRejectsInvalidInput(t *testing.T) {
	now := time.Date(2026, 7, 28, 3, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	validHash := strings.Repeat("a", 64)

	tests := []struct {
		name        string
		userID      uint64
		keyHash     string
		requestHash string
		expiresAt   time.Time
		now         time.Time
	}{
		{name: "zero user", userID: 0, keyHash: validHash, requestHash: validHash, expiresAt: now.Add(time.Hour), now: now},
		{name: "short key hash", userID: 1, keyHash: strings.Repeat("a", 63), requestHash: validHash, expiresAt: now.Add(time.Hour), now: now},
		{name: "uppercase key hash", userID: 1, keyHash: strings.Repeat("A", 64), requestHash: validHash, expiresAt: now.Add(time.Hour), now: now},
		{name: "non hex key hash", userID: 1, keyHash: strings.Repeat("g", 64), requestHash: validHash, expiresAt: now.Add(time.Hour), now: now},
		{name: "invalid request hash", userID: 1, keyHash: validHash, requestHash: strings.Repeat("z", 64), expiresAt: now.Add(time.Hour), now: now},
		{name: "zero now", userID: 1, keyHash: validHash, requestHash: validHash, expiresAt: now.Add(time.Hour), now: time.Time{}},
		{name: "zero expiry", userID: 1, keyHash: validHash, requestHash: validHash, expiresAt: time.Time{}, now: now},
		{name: "expiry equals now", userID: 1, keyHash: validHash, requestHash: validHash, expiresAt: now, now: now},
		{name: "expiry before now", userID: 1, keyHash: validHash, requestHash: validHash, expiresAt: now.Add(-time.Second), now: now},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewIdempotencyRecord(tt.userID, tt.keyHash, tt.requestHash, tt.expiresAt, tt.now); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestIdempotencyScopeValidation(t *testing.T) {
	if !IdempotencyScopeVideoCreate.IsValid() {
		t.Fatal("video_create must be valid")
	}
	if IdempotencyScope("invalid").IsValid() {
		t.Fatal("unexpected valid scope")
	}
}
