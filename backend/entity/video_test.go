package entity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewVideoInitialState(t *testing.T) {
	now := testVideoTime()
	video, err := NewVideo(10, CategoryBrewing, "  蒸らし方  ", "  説明  ", "  videos/pending/source.mp4  ", now.Add(15*time.Minute), now)
	if err != nil {
		t.Fatalf("NewVideo returned error: %v", err)
	}
	if video.Title != "蒸らし方" || video.Description != "説明" || video.OriginalObjectKey != "videos/pending/source.mp4" {
		t.Fatalf("input was not normalized: %#v", video)
	}
	if video.ProcessingStatus != VideoProcessingUploading || video.PublishStatus != VideoPublishPrivate {
		t.Fatalf("unexpected initial state: %s/%s", video.ProcessingStatus, video.PublishStatus)
	}
	if video.SourceMeta != nil || video.OutputMeta != nil || video.DeletedAt != nil {
		t.Fatal("new video must not have meta or deleted_at")
	}
	if !video.CreatedAt.Equal(now) || !video.UpdatedAt.Equal(now) || !video.UploadExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("unexpected timestamps: %#v", video)
	}
}

func TestNewVideoRejectsInvalidInput(t *testing.T) {
	now := testVideoTime()
	validTitle := "抽出"
	validDescription := "説明"
	validObjectKey := "videos/1/source/a.mp4"
	validExpiry := now.Add(15 * time.Minute)

	tests := []struct {
		name        string
		userID      uint64
		category    CategoryCode
		title       string
		description string
		objectKey   string
		expiresAt   time.Time
		now         time.Time
	}{
		{name: "zero user", userID: 0, category: CategoryBrewing, title: validTitle, description: validDescription, objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "invalid category", userID: 1, category: CategoryCode("invalid"), title: validTitle, description: validDescription, objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "empty title", userID: 1, category: CategoryBrewing, title: "   ", description: validDescription, objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "title too long", userID: 1, category: CategoryBrewing, title: strings.Repeat("あ", 101), description: validDescription, objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "invalid title utf8", userID: 1, category: CategoryBrewing, title: string([]byte{0xff}), description: validDescription, objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "description too long", userID: 1, category: CategoryBrewing, title: validTitle, description: strings.Repeat("あ", 1001), objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "invalid description utf8", userID: 1, category: CategoryBrewing, title: validTitle, description: string([]byte{0xff}), objectKey: validObjectKey, expiresAt: validExpiry, now: now},
		{name: "empty object key", userID: 1, category: CategoryBrewing, title: validTitle, description: validDescription, objectKey: "   ", expiresAt: validExpiry, now: now},
		{name: "zero now", userID: 1, category: CategoryBrewing, title: validTitle, description: validDescription, objectKey: validObjectKey, expiresAt: validExpiry, now: time.Time{}},
		{name: "zero expiry", userID: 1, category: CategoryBrewing, title: validTitle, description: validDescription, objectKey: validObjectKey, expiresAt: time.Time{}, now: now},
		{name: "expiry equals now", userID: 1, category: CategoryBrewing, title: validTitle, description: validDescription, objectKey: validObjectKey, expiresAt: now, now: now},
		{name: "expiry before now", userID: 1, category: CategoryBrewing, title: validTitle, description: validDescription, objectKey: validObjectKey, expiresAt: now.Add(-time.Second), now: now},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewVideo(tt.userID, tt.category, tt.title, tt.description, tt.objectKey, tt.expiresAt, tt.now); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestVideoProcessingLifecyclePublishesForActiveOwner(t *testing.T) {
	now := testVideoTime()
	video := mustNewTestVideo(t, now)
	video.ID = 1

	if err := video.CompleteUpload(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.StartProcessing(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.RecordSourceValidation(validSourceMeta(), now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.CompleteProcessing(validOutputMeta(), true, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !video.CanBeViewedPublicly() {
		t.Fatalf("completed active owner video must be public: %s/%s", video.ProcessingStatus, video.PublishStatus)
	}
	if video.OutputMeta == nil || video.OutputMeta.VideoID != video.ID {
		t.Fatalf("output meta was not attached correctly: %#v", video.OutputMeta)
	}
}

func TestVideoCompleteProcessingKeepsSuspendedOwnerPrivate(t *testing.T) {
	now := testVideoTime()
	video := mustProcessingVideo(t, now)

	if err := video.CompleteProcessing(validOutputMeta(), false, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if video.ProcessingStatus != VideoProcessingReady || video.PublishStatus != VideoPublishPrivate {
		t.Fatalf("unexpected suspended owner state: %s/%s", video.ProcessingStatus, video.PublishStatus)
	}
}

func TestVideoStateMethodsRejectZeroTime(t *testing.T) {
	now := testVideoTime()

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "complete upload", run: func() error { return mustNewTestVideo(t, now).CompleteUpload(time.Time{}) }},
		{name: "expire upload", run: func() error { return mustNewTestVideo(t, now).ExpireUpload(time.Time{}) }},
		{name: "start processing", run: func() error {
			video := mustNewTestVideo(t, now)
			video.ProcessingStatus = VideoProcessingUploaded
			return video.StartProcessing(time.Time{})
		}},
		{name: "record source", run: func() error {
			return mustProcessingVideo(t, now).RecordSourceValidation(validSourceMeta(), time.Time{})
		}},
		{name: "complete processing", run: func() error {
			return mustProcessingVideo(t, now).CompleteProcessing(validOutputMeta(), true, time.Time{})
		}},
		{name: "fail processing", run: func() error { return mustProcessingVideo(t, now).FailProcessing(time.Time{}) }},
		{name: "set private", run: func() error {
			video := readyPublishedVideo(now)
			return video.SetPrivateByOwner(video.UserID, time.Time{})
		}},
		{name: "republish", run: func() error {
			video := readyPrivateVideo(now)
			return video.RepublishByOwner(video.UserID, true, time.Time{})
		}},
		{name: "hide admin", run: func() error { return readyPublishedVideo(now).HideByAdmin(time.Time{}) }},
		{name: "restore admin", run: func() error { return readyHiddenVideo(now).RestoreByAdmin(true, time.Time{}) }},
		{name: "delete owner", run: func() error {
			video := readyPublishedVideo(now)
			return video.DeleteByOwner(video.UserID, time.Time{})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestVideoUploadExpirationBoundary(t *testing.T) {
	now := testVideoTime()
	video := mustNewTestVideo(t, now)
	if err := video.CompleteUpload(video.UploadExpiresAt); !errors.Is(err, ErrUploadExpired) {
		t.Fatalf("expected ErrUploadExpired, got %v", err)
	}

	video = mustNewTestVideo(t, now)
	if err := video.ExpireUpload(video.UploadExpiresAt.Add(-time.Nanosecond)); !errors.Is(err, ErrVideoStateConflict) {
		t.Fatalf("expected ErrVideoStateConflict before expiry, got %v", err)
	}
	if err := video.ExpireUpload(video.UploadExpiresAt); err != nil {
		t.Fatalf("expected expiration at boundary to succeed, got %v", err)
	}
	if video.ProcessingStatus != VideoProcessingExpired {
		t.Fatalf("expected expired status, got %s", video.ProcessingStatus)
	}
}

func TestVideoSourceAndOutputCannotBeRecordedTwice(t *testing.T) {
	now := testVideoTime()
	video := mustProcessingVideo(t, now)
	if err := video.RecordSourceValidation(validSourceMeta(), now.Add(3*time.Minute)); !errors.Is(err, ErrVideoStateConflict) {
		t.Fatalf("expected duplicate source conflict, got %v", err)
	}

	video.OutputMeta = &OutputVideoMeta{}
	if err := video.CompleteProcessing(validOutputMeta(), true, now.Add(4*time.Minute)); !errors.Is(err, ErrVideoStateConflict) {
		t.Fatalf("expected duplicate output conflict, got %v", err)
	}
}

func TestVideoOwnerAndAdminTransitions(t *testing.T) {
	now := testVideoTime()
	video := readyPublishedVideo(now)

	if err := video.SetPrivateByOwner(999, now.Add(time.Minute)); !errors.Is(err, ErrVideoForbidden) {
		t.Fatalf("expected ErrVideoForbidden, got %v", err)
	}
	if err := video.SetPrivateByOwner(video.UserID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.RepublishByOwner(video.UserID, false, now.Add(2*time.Minute)); !errors.Is(err, ErrUserSuspended) {
		t.Fatalf("expected ErrUserSuspended, got %v", err)
	}
	if err := video.RepublishByOwner(video.UserID, true, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.HideByAdmin(now.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.RepublishByOwner(video.UserID, true, now.Add(4*time.Minute)); !errors.Is(err, ErrVideoStateConflict) {
		t.Fatalf("owner must not republish hidden video: %v", err)
	}
	if err := video.RestoreByAdmin(false, now.Add(4*time.Minute)); !errors.Is(err, ErrUserSuspended) {
		t.Fatalf("expected ErrUserSuspended, got %v", err)
	}
	if err := video.RestoreByAdmin(true, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func TestVideoDeleteByOwner(t *testing.T) {
	now := testVideoTime()
	video := readyPublishedVideo(now)

	if err := video.DeleteByOwner(999, now.Add(time.Minute)); !errors.Is(err, ErrVideoForbidden) {
		t.Fatalf("expected ErrVideoForbidden, got %v", err)
	}
	if err := video.DeleteByOwner(video.UserID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if video.DeletedAt == nil || video.PublishStatus != VideoPublishPrivate || video.CanBeViewedPublicly() {
		t.Fatalf("unexpected deleted video: %#v", video)
	}
	if err := video.DeleteByOwner(video.UserID, now.Add(2*time.Minute)); !errors.Is(err, ErrVideoStateConflict) {
		t.Fatalf("expected duplicate deletion conflict, got %v", err)
	}
}

func TestSourceVideoMetaValidate(t *testing.T) {
	tests := []struct {
		name string
		edit func(*SourceVideoMeta)
	}{
		{name: "mismatched mime and container", edit: func(meta *SourceVideoMeta) { meta.MIMEType = "video/quicktime" }},
		{name: "size zero", edit: func(meta *SourceVideoMeta) { meta.SizeBytes = 0 }},
		{name: "size exceeded", edit: func(meta *SourceVideoMeta) { meta.SizeBytes = 30_000_001 }},
		{name: "duration zero", edit: func(meta *SourceVideoMeta) { meta.DurationMillis = 0 }},
		{name: "duration exceeded", edit: func(meta *SourceVideoMeta) { meta.DurationMillis = 10_001 }},
		{name: "width exceeded", edit: func(meta *SourceVideoMeta) { meta.Width = 1081 }},
		{name: "height exceeded", edit: func(meta *SourceVideoMeta) { meta.Height = 1921 }},
		{name: "invalid aspect", edit: func(meta *SourceVideoMeta) { meta.Width = 1280; meta.Height = 720 }},
		{name: "frame rate zero", edit: func(meta *SourceVideoMeta) { meta.FrameRate = 0 }},
		{name: "frame rate exceeded", edit: func(meta *SourceVideoMeta) { meta.FrameRate = 60.1 }},
		{name: "video codec empty", edit: func(meta *SourceVideoMeta) { meta.VideoCodec = "   " }},
		{name: "audio codec missing", edit: func(meta *SourceVideoMeta) { meta.HasAudio = true; meta.AudioCodec = "" }},
		{name: "audio codec present without track", edit: func(meta *SourceVideoMeta) { meta.HasAudio = false; meta.AudioCodec = "aac" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := validSourceMeta()
			tt.edit(&meta)
			if err := meta.Validate(); !errors.Is(err, ErrVideoSourceInvalid) {
				t.Fatalf("expected ErrVideoSourceInvalid, got %v", err)
			}
		})
	}

	mov := validSourceMeta()
	mov.MIMEType = "video/quicktime"
	mov.Container = "mov"
	if err := mov.Validate(); err != nil {
		t.Fatalf("valid MOV was rejected: %v", err)
	}
}

func TestOutputVideoMetaValidate(t *testing.T) {
	tests := []struct {
		name string
		edit func(*OutputVideoMeta)
	}{
		{name: "video key empty", edit: func(meta *OutputVideoMeta) { meta.VideoObjectKey = "   " }},
		{name: "thumbnail key empty", edit: func(meta *OutputVideoMeta) { meta.ThumbnailObjectKey = "" }},
		{name: "same object key", edit: func(meta *OutputVideoMeta) { meta.ThumbnailObjectKey = meta.VideoObjectKey }},
		{name: "wrong container", edit: func(meta *OutputVideoMeta) { meta.Container = "mov" }},
		{name: "wrong width", edit: func(meta *OutputVideoMeta) { meta.Width = 1080 }},
		{name: "wrong height", edit: func(meta *OutputVideoMeta) { meta.Height = 1920 }},
		{name: "frame rate zero", edit: func(meta *OutputVideoMeta) { meta.FrameRate = 0 }},
		{name: "frame rate exceeded", edit: func(meta *OutputVideoMeta) { meta.FrameRate = 30.1 }},
		{name: "wrong video codec", edit: func(meta *OutputVideoMeta) { meta.VideoCodec = "hevc" }},
		{name: "wrong audio codec", edit: func(meta *OutputVideoMeta) { meta.AudioCodec = "mp3" }},
		{name: "audio codec without track", edit: func(meta *OutputVideoMeta) { meta.HasAudio = false; meta.AudioCodec = "aac" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := validOutputMeta()
			tt.edit(&meta)
			if err := meta.Validate(); !errors.Is(err, ErrVideoOutputInvalid) {
				t.Fatalf("expected ErrVideoOutputInvalid, got %v", err)
			}
		})
	}

	withoutAudio := validOutputMeta()
	withoutAudio.HasAudio = false
	withoutAudio.AudioCodec = ""
	if err := withoutAudio.Validate(); err != nil {
		t.Fatalf("valid silent output was rejected: %v", err)
	}
}

func testVideoTime() time.Time {
	return time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC)
}

func mustNewTestVideo(t *testing.T, now time.Time) *Video {
	t.Helper()
	video, err := NewVideo(10, CategoryLatteArt, "ラテアート", "", "videos/1/source/a.mp4", now.Add(15*time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	video.ID = 1
	return video
}

func mustProcessingVideo(t *testing.T, now time.Time) *Video {
	t.Helper()
	video := mustNewTestVideo(t, now)
	if err := video.CompleteUpload(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.StartProcessing(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := video.RecordSourceValidation(validSourceMeta(), now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return video
}

func validSourceMeta() SourceVideoMeta {
	return SourceVideoMeta{
		MIMEType:       "video/mp4",
		Container:      "mp4",
		SizeBytes:      1_000_000,
		DurationMillis: 5_000,
		Width:          720,
		Height:         1280,
		FrameRate:      30,
		VideoCodec:     "h264",
		HasAudio:       true,
		AudioCodec:     "aac",
	}
}

func validOutputMeta() OutputVideoMeta {
	return OutputVideoMeta{
		VideoObjectKey:     "videos/1/output/a.mp4",
		ThumbnailObjectKey: "videos/1/thumbnail/a.jpg",
		Container:          "mp4",
		Width:              720,
		Height:             1280,
		FrameRate:          30,
		VideoCodec:         "h264",
		HasAudio:           true,
		AudioCodec:         "aac",
	}
}

func readyPublishedVideo(now time.Time) *Video {
	return &Video{ID: 1, UserID: 10, ProcessingStatus: VideoProcessingReady, PublishStatus: VideoPublishPublished, CreatedAt: now, UpdatedAt: now}
}

func readyPrivateVideo(now time.Time) *Video {
	return &Video{ID: 1, UserID: 10, ProcessingStatus: VideoProcessingReady, PublishStatus: VideoPublishPrivate, CreatedAt: now, UpdatedAt: now}
}

func readyHiddenVideo(now time.Time) *Video {
	return &Video{ID: 1, UserID: 10, ProcessingStatus: VideoProcessingReady, PublishStatus: VideoPublishHidden, CreatedAt: now, UpdatedAt: now}
}
