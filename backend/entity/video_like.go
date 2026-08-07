package entity

import "time"

type VideoLike struct {
	ID        uint64    `json:"id" gorm:"primaryKey;index:idx_video_likes_user,priority:3,sort:desc"`
	UserID    uint64    `json:"user_id" gorm:"not null;uniqueIndex:uq_video_likes_user_video,priority:1;index:idx_video_likes_user,priority:1"`
	VideoID   uint64    `json:"video_id" gorm:"not null;uniqueIndex:uq_video_likes_user_video,priority:2;index:idx_video_likes_video,priority:1"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;index:idx_video_likes_video,priority:2,sort:desc;index:idx_video_likes_user,priority:2,sort:desc"`
}

func NewVideoLike(userID, videoID uint64, now time.Time) (*VideoLike, error) {
	if userID == 0 || videoID == 0 || now.IsZero() {
		return nil, ErrInvalidInput
	}

	return &VideoLike{
		UserID:    userID,
		VideoID:   videoID,
		CreatedAt: now,
	}, nil
}
