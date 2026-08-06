package entity

import "time"

type VideoLike struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	UserID    uint64    `json:"user_id" gorm:"not null;uniqueIndex:uq_video_likes_user_video,priority:1;index:idx_video_likes_user"`
	VideoID   uint64    `json:"video_id" gorm:"not null;uniqueIndex:uq_video_likes_user_video,priority:2;index:idx_video_likes_video"`
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
}

func NewVideoLike(userID, videoID uint64, now time.Time) (*VideoLike, error) {
	if userID == 0 || videoID == 0 || now.IsZero() {
		return nil, ErrInvalidInput
	}

	return &VideoLike{
		UserID:    userID,
		VideoID:   videoID,
		CreatedAt: now.UTC(),
	}, nil
}
