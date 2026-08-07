package entity

import (
	"errors"
	"testing"
	"time"
)

func TestNewVideoLike(t *testing.T) {
	now := time.Date(2026, 8, 6, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	like, err := NewVideoLike(10, 20, now)
	if err != nil {
		t.Fatalf("NewVideoLike() error = %v", err)
	}
	if like.UserID != 10 || like.VideoID != 20 {
		t.Fatalf("like = %#v", like)
	}
	if !like.CreatedAt.Equal(now) || like.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt = %v, want UTC", like.CreatedAt)
	}
}

func TestNewVideoLikeRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		name    string
		userID  uint64
		videoID uint64
		now     time.Time
	}{
		{name: "zero user", userID: 0, videoID: 1, now: now},
		{name: "zero video", userID: 1, videoID: 0, now: now},
		{name: "zero time", userID: 1, videoID: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			like, err := NewVideoLike(tt.userID, tt.videoID, tt.now)
			if !errors.Is(err, ErrInvalidInput) || like != nil {
				t.Fatalf("like=%#v error=%v", like, err)
			}
		})
	}
}
