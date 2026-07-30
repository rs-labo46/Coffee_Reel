package entity

import "time"

type SavedVideo struct {
	ID        uint64    `json:"id" gorm:"primaryKey;index:idx_saved_videos_user_list,sort:desc,priority:3"`
	UserID    uint64    `json:"user_id" gorm:"not null;uniqueIndex:uq_saved_videos_user_video,priority:1;index:idx_saved_videos_user_list,priority:1"`
	VideoID   uint64    `json:"video_id" gorm:"not null;uniqueIndex:uq_saved_videos_user_video,priority:2;index:idx_saved_videos_video"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;index:idx_saved_videos_user_list,sort:desc,priority:2"`
}

func NewSavedVideo(userID, videoID uint64, now time.Time) (*SavedVideo, error) {
	if userID == 0 || videoID == 0 || now.IsZero() {
		return nil, ErrInvalidInput
	}
	return &SavedVideo{UserID: userID, VideoID: videoID, CreatedAt: now.UTC()}, nil
}
