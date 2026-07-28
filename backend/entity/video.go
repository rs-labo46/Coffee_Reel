package entity

import (
	"strings"
	"time"
	"unicode/utf8"
)

type VideoProcessingStatus string

const (
	VideoProcessingUploading  VideoProcessingStatus = "uploading"
	VideoProcessingExpired    VideoProcessingStatus = "expired"
	VideoProcessingUploaded   VideoProcessingStatus = "uploaded"
	VideoProcessingProcessing VideoProcessingStatus = "processing"
	VideoProcessingReady      VideoProcessingStatus = "ready"
	VideoProcessingFailed     VideoProcessingStatus = "failed"
)

type VideoPublishStatus string

const (
	VideoPublishPrivate   VideoPublishStatus = "private"
	VideoPublishPublished VideoPublishStatus = "published"
	VideoPublishHidden    VideoPublishStatus = "hidden"
)

type Video struct {
	ID                uint64                `json:"id" gorm:"primaryKey"`
	UserID            uint64                `json:"user_id" gorm:"not null;index:idx_videos_owner_list,priority:1"`
	Category          CategoryCode          `json:"category" gorm:"type:varchar(32);not null"`
	Title             string                `json:"title" gorm:"type:varchar(100);not null"`
	Description       string                `json:"description" gorm:"type:varchar(1000);not null"`
	OriginalObjectKey string                `json:"-" gorm:"type:varchar(1024);not null;uniqueIndex:uq_videos_original_object_key"`
	UploadExpiresAt   time.Time             `json:"upload_expires_at" gorm:"not null;index:idx_videos_upload_expiration,priority:2"`
	ProcessingStatus  VideoProcessingStatus `json:"processing_status" gorm:"type:varchar(32);not null;index:idx_videos_upload_expiration,priority:1"`
	PublishStatus     VideoPublishStatus    `json:"publish_status" gorm:"type:varchar(32);not null"`
	CreatedAt         time.Time             `json:"created_at" gorm:"not null;index:idx_videos_owner_list,sort:desc,priority:2"`
	UpdatedAt         time.Time             `json:"updated_at" gorm:"not null"`
	DeletedAt         *time.Time            `json:"deleted_at" gorm:"index"`
	SourceMeta        *SourceVideoMeta      `json:"source_meta,omitempty" gorm:"-"`
	OutputMeta        *OutputVideoMeta      `json:"output_meta,omitempty" gorm:"-"`
}

type SourceVideoMeta struct {
	ID             uint64    `json:"id" gorm:"primaryKey"`
	VideoID        uint64    `json:"video_id" gorm:"not null;uniqueIndex:uq_video_source_metas_video_id"`
	MIMEType       string    `json:"mime_type" gorm:"type:varchar(64);not null"`
	Container      string    `json:"container" gorm:"type:varchar(16);not null"`
	SizeBytes      int64     `json:"size_bytes" gorm:"not null"`
	DurationMillis int64     `json:"duration_millis" gorm:"not null"`
	Width          int       `json:"width" gorm:"not null"`
	Height         int       `json:"height" gorm:"not null"`
	FrameRate      float64   `json:"frame_rate" gorm:"not null"`
	VideoCodec     string    `json:"video_codec" gorm:"type:varchar(32);not null"`
	HasAudio       bool      `json:"has_audio" gorm:"not null"`
	AudioCodec     string    `json:"audio_codec" gorm:"type:varchar(32);not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"not null"`
}

func (SourceVideoMeta) TableName() string {
	return "video_source_metas"
}

type OutputVideoMeta struct {
	ID                 uint64    `json:"id" gorm:"primaryKey"`
	VideoID            uint64    `json:"video_id" gorm:"not null;uniqueIndex:uq_video_output_metas_video_id"`
	VideoObjectKey     string    `json:"-" gorm:"type:varchar(1024);not null;uniqueIndex:uq_video_output_metas_video_object_key"`
	ThumbnailObjectKey string    `json:"-" gorm:"type:varchar(1024);not null;uniqueIndex:uq_video_output_metas_thumbnail_object_key"`
	Container          string    `json:"container" gorm:"type:varchar(16);not null"`
	Width              int       `json:"width" gorm:"not null"`
	Height             int       `json:"height" gorm:"not null"`
	FrameRate          float64   `json:"frame_rate" gorm:"not null"`
	VideoCodec         string    `json:"video_codec" gorm:"type:varchar(32);not null"`
	HasAudio           bool      `json:"has_audio" gorm:"not null"`
	AudioCodec         string    `json:"audio_codec" gorm:"type:varchar(32);not null"`
	CreatedAt          time.Time `json:"created_at" gorm:"not null"`
}

func (OutputVideoMeta) TableName() string {
	return "video_output_metas"
}

func NewVideo(userID uint64, category CategoryCode, title, description, originalObjectKey string, uploadExpiresAt, now time.Time) (*Video, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	originalObjectKey = strings.TrimSpace(originalObjectKey)
	now = now.UTC()
	uploadExpiresAt = uploadExpiresAt.UTC()

	if userID == 0 || !category.IsValid() || now.IsZero() || !uploadExpiresAt.After(now) {
		return nil, ErrInvalidInput
	}
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 100 {
		return nil, ErrInvalidInput
	}
	if !utf8.ValidString(description) || utf8.RuneCountInString(description) > 1000 || originalObjectKey == "" {
		return nil, ErrInvalidInput
	}

	return &Video{
		UserID:            userID,
		Category:          category,
		Title:             title,
		Description:       description,
		OriginalObjectKey: originalObjectKey,
		UploadExpiresAt:   uploadExpiresAt,
		ProcessingStatus:  VideoProcessingUploading,
		PublishStatus:     VideoPublishPrivate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (v *Video) IsOwnedBy(userID uint64) bool {
	return v != nil && userID != 0 && v.UserID == userID
}

func (v *Video) IsDeleted() bool {
	return v == nil || v.DeletedAt != nil
}

func (v *Video) CanBeViewedPublicly() bool {
	return v != nil && v.DeletedAt == nil && v.ProcessingStatus == VideoProcessingReady && v.PublishStatus == VideoPublishPublished
}

func (v *Video) CompleteUpload(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	now = now.UTC()
	if v == nil || v.IsDeleted() {
		return ErrVideoStateConflict
	}
	if v.ProcessingStatus != VideoProcessingUploading || v.PublishStatus != VideoPublishPrivate {
		return ErrVideoStateConflict
	}
	if !now.Before(v.UploadExpiresAt.UTC()) {
		return ErrUploadExpired
	}
	v.ProcessingStatus = VideoProcessingUploaded
	v.UpdatedAt = now
	return nil
}

func (v *Video) ExpireUpload(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	now = now.UTC()
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingUploading || v.PublishStatus != VideoPublishPrivate {
		return ErrVideoStateConflict
	}
	if now.Before(v.UploadExpiresAt.UTC()) {
		return ErrVideoStateConflict
	}
	v.ProcessingStatus = VideoProcessingExpired
	v.UpdatedAt = now
	return nil
}

func (v *Video) StartProcessing(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingUploaded || v.PublishStatus != VideoPublishPrivate {
		return ErrVideoStateConflict
	}
	v.ProcessingStatus = VideoProcessingProcessing
	v.UpdatedAt = now.UTC()
	return nil
}

func (v *Video) RecordSourceValidation(meta SourceVideoMeta, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingProcessing || v.PublishStatus != VideoPublishPrivate || v.SourceMeta != nil {
		return ErrVideoStateConflict
	}
	if err := meta.Validate(); err != nil {
		return err
	}
	now = now.UTC()
	meta.VideoID = v.ID
	meta.CreatedAt = now
	v.SourceMeta = &meta
	v.UpdatedAt = now
	return nil
}

func (v *Video) CompleteProcessing(meta OutputVideoMeta, ownerActive bool, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingProcessing || v.PublishStatus != VideoPublishPrivate || v.SourceMeta == nil || v.OutputMeta != nil {
		return ErrVideoStateConflict
	}
	if err := meta.Validate(); err != nil {
		return err
	}
	now = now.UTC()
	meta.VideoID = v.ID
	meta.CreatedAt = now
	v.OutputMeta = &meta
	v.ProcessingStatus = VideoProcessingReady
	if ownerActive {
		v.PublishStatus = VideoPublishPublished
	} else {
		v.PublishStatus = VideoPublishPrivate
	}
	v.UpdatedAt = now
	return nil
}

func (v *Video) FailProcessing(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingProcessing || v.PublishStatus != VideoPublishPrivate {
		return ErrVideoStateConflict
	}
	v.ProcessingStatus = VideoProcessingFailed
	v.PublishStatus = VideoPublishPrivate
	v.UpdatedAt = now.UTC()
	return nil
}

func (v *Video) SetPrivateByOwner(userID uint64, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || !v.IsOwnedBy(userID) {
		return ErrVideoForbidden
	}
	if v.IsDeleted() || v.ProcessingStatus != VideoProcessingReady || v.PublishStatus != VideoPublishPublished {
		return ErrVideoStateConflict
	}
	v.PublishStatus = VideoPublishPrivate
	v.UpdatedAt = now.UTC()
	return nil
}

func (v *Video) RepublishByOwner(userID uint64, ownerActive bool, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || !v.IsOwnedBy(userID) {
		return ErrVideoForbidden
	}
	if v.IsDeleted() || v.ProcessingStatus != VideoProcessingReady || v.PublishStatus != VideoPublishPrivate {
		return ErrVideoStateConflict
	}
	if !ownerActive {
		return ErrUserSuspended
	}
	v.PublishStatus = VideoPublishPublished
	v.UpdatedAt = now.UTC()
	return nil
}

func (v *Video) HideByAdmin(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingReady || v.PublishStatus != VideoPublishPublished {
		return ErrVideoStateConflict
	}
	v.PublishStatus = VideoPublishHidden
	v.UpdatedAt = now.UTC()
	return nil
}

func (v *Video) RestoreByAdmin(ownerActive bool, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || v.IsDeleted() || v.ProcessingStatus != VideoProcessingReady || v.PublishStatus != VideoPublishHidden {
		return ErrVideoStateConflict
	}
	if !ownerActive {
		return ErrUserSuspended
	}
	v.PublishStatus = VideoPublishPublished
	v.UpdatedAt = now.UTC()
	return nil
}

func (v *Video) DeleteByOwner(userID uint64, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidInput
	}
	if v == nil || !v.IsOwnedBy(userID) {
		return ErrVideoForbidden
	}
	if v.IsDeleted() {
		return ErrVideoStateConflict
	}
	deletedAt := now.UTC()
	v.DeletedAt = &deletedAt
	v.PublishStatus = VideoPublishPrivate
	v.UpdatedAt = deletedAt
	return nil
}

func (m SourceVideoMeta) Validate() error {
	validFormat := (m.MIMEType == "video/mp4" && m.Container == "mp4") ||
		(m.MIMEType == "video/quicktime" && m.Container == "mov")
	if !validFormat {
		return ErrVideoSourceInvalid
	}
	if m.SizeBytes < 1 || m.SizeBytes > 30_000_000 || m.DurationMillis < 1 || m.DurationMillis > 10_000 {
		return ErrVideoSourceInvalid
	}
	if m.Width < 1 || m.Width > 1080 || m.Height < 1 || m.Height > 1920 || m.Width*16 != m.Height*9 {
		return ErrVideoSourceInvalid
	}
	if m.FrameRate <= 0 || m.FrameRate > 60 || strings.TrimSpace(m.VideoCodec) == "" {
		return ErrVideoSourceInvalid
	}
	if !m.HasAudio && strings.TrimSpace(m.AudioCodec) != "" {
		return ErrVideoSourceInvalid
	}
	if m.HasAudio && strings.TrimSpace(m.AudioCodec) == "" {
		return ErrVideoSourceInvalid
	}
	return nil
}

func (m OutputVideoMeta) Validate() error {
	videoObjectKey := strings.TrimSpace(m.VideoObjectKey)
	thumbnailObjectKey := strings.TrimSpace(m.ThumbnailObjectKey)
	if videoObjectKey == "" || thumbnailObjectKey == "" || videoObjectKey == thumbnailObjectKey {
		return ErrVideoOutputInvalid
	}
	if m.Container != "mp4" || m.Width != 720 || m.Height != 1280 || m.FrameRate <= 0 || m.FrameRate > 30 || m.VideoCodec != "h264" {
		return ErrVideoOutputInvalid
	}
	if m.HasAudio && m.AudioCodec != "aac" {
		return ErrVideoOutputInvalid
	}
	if !m.HasAudio && m.AudioCodec != "" {
		return ErrVideoOutputInvalid
	}
	return nil
}
