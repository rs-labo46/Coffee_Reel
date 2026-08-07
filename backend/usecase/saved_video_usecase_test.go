package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

func TestNewSavedVideoUsecaseRejectsInvalidConfiguration(t *testing.T) {
	validSaved := &savedVideoRepositoryMock{}
	validStorage := &objectStorageRepositoryMock{}
	tests := []struct {
		name    string
		saved   repository.ISavedVideoRepository
		storage repository.IObjectStorageRepository
		ttl     time.Duration
	}{
		{name: "saved repository missing", storage: validStorage, ttl: time.Minute},
		{name: "storage repository missing", saved: validSaved, ttl: time.Minute},
		{name: "read ttl zero", saved: validSaved, storage: validStorage, ttl: 0},
		{name: "read ttl negative", saved: validSaved, storage: validStorage, ttl: -time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSavedVideoUsecase(tt.saved, tt.storage, SavedVideoUsecaseConfig{ReadURLTTL: tt.ttl}); err == nil {
				t.Fatal("NewSavedVideoUsecase() error=nil")
			}
		})
	}
}

func TestSavedVideoUsecaseSaveValidatesActorAndForwardsUTC(t *testing.T) {
	before := time.Now()
	called := false
	saved := &savedVideoRepositoryMock{saveFunc: func(_ context.Context, userID, videoID uint64, now time.Time) error {
		called = true
		if userID != 4 || videoID != 9 {
			t.Fatalf("Save(%d,%d)", userID, videoID)
		}
		if now.Location() != time.UTC || now.Before(before) || now.After(time.Now()) {
			t.Fatalf("now=%s", now)
		}
		return nil
	}}
	uc, _ := NewSavedVideoUsecase(saved, &objectStorageRepositoryMock{}, SavedVideoUsecaseConfig{ReadURLTTL: 10 * time.Minute})
	if err := uc.Save(context.Background(), activeVideoUser(4), 9); err != nil {
		t.Fatalf("Save() error=%v", err)
	}
	if !called {
		t.Fatal("repository Save was not called")
	}

	tests := []struct {
		name    string
		actor   *entity.User
		videoID uint64
		wantErr error
	}{
		{name: "unauthorized", actor: nil, videoID: 9, wantErr: entity.ErrUnauthorized},
		{name: "suspended", actor: suspendedVideoUser(4), videoID: 9, wantErr: entity.ErrUserSuspended},
		{name: "zero video", actor: activeVideoUser(4), videoID: 0, wantErr: entity.ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noCall := &savedVideoRepositoryMock{saveFunc: func(context.Context, uint64, uint64, time.Time) error {
				t.Fatal("repository Save called for invalid input")
				return nil
			}}
			testUC, _ := NewSavedVideoUsecase(noCall, &objectStorageRepositoryMock{}, SavedVideoUsecaseConfig{ReadURLTTL: time.Minute})
			if err := testUC.Save(context.Background(), tt.actor, tt.videoID); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v want=%v", err, tt.wantErr)
			}
		})
	}
}

func TestSavedVideoUsecaseSaveTreatsRepositoryDuplicateAsSuccess(t *testing.T) {
	saved := &savedVideoRepositoryMock{saveFunc: func(context.Context, uint64, uint64, time.Time) error { return nil }}
	uc, _ := NewSavedVideoUsecase(saved, &objectStorageRepositoryMock{}, SavedVideoUsecaseConfig{ReadURLTTL: time.Minute})
	if err := uc.Save(context.Background(), activeVideoUser(1), 2); err != nil {
		t.Fatalf("Save() error=%v", err)
	}
}

func TestSavedVideoUsecaseRemoveIsIdempotentAndValidatesInput(t *testing.T) {
	calls := 0
	saved := &savedVideoRepositoryMock{removeFunc: func(_ context.Context, userID, videoID uint64) error {
		calls++
		if userID != 6 || videoID != 7 {
			t.Fatalf("Remove(%d,%d)", userID, videoID)
		}
		return nil
	}}
	uc, _ := NewSavedVideoUsecase(saved, &objectStorageRepositoryMock{}, SavedVideoUsecaseConfig{ReadURLTTL: time.Minute})
	if err := uc.Remove(context.Background(), activeVideoUser(6), 7); err != nil {
		t.Fatalf("first Remove() error=%v", err)
	}
	if err := uc.Remove(context.Background(), activeVideoUser(6), 7); err != nil {
		t.Fatalf("replayed Remove() error=%v", err)
	}
	if calls != 2 {
		t.Fatalf("remove calls=%d want=2", calls)
	}
	if err := uc.Remove(context.Background(), activeVideoUser(6), 0); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("zero id error=%v", err)
	}
}

func TestSavedVideoUsecaseListBuildsReadURLsAndSavedCursor(t *testing.T) {
	created := time.Date(2026, 8, 3, 9, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	inputCursor := &VideoCursor{CreatedAt: created.Add(time.Hour), ID: 99}
	saved := &savedVideoRepositoryMock{listByUserFunc: func(_ context.Context, userID uint64, limit int, cursor *repository.SavedVideoCursor) (repository.SavedVideoPage, error) {
		if userID != 3 || limit != 2 || cursor == nil || cursor.ID != 99 || cursor.CreatedAt.Location() != time.UTC {
			t.Fatalf("ListByUser(%d,%d,%+v)", userID, limit, cursor)
		}
		return repository.SavedVideoPage{
			Items:   []repository.PublicVideoItem{{ID: 11, UserID: 8, AuthorName: "author", Category: entity.CategoryLatteArt, Title: "latte", Description: "desc", VideoObjectKey: "videos/11/output/a.mp4", ThumbnailObjectKey: "videos/11/thumbnail/a.jpg", CreatedAt: created, IsSaved: true}},
			HasMore: true, LastCreatedAt: created, LastID: 55,
		}, nil
	}}
	readCalls := []string{}
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(_ context.Context, key string, ttl time.Duration) (repository.ReadTarget, error) {
		readCalls = append(readCalls, key)
		if ttl != 10*time.Minute {
			t.Fatalf("ttl=%s", ttl)
		}
		return repository.ReadTarget{URL: "https://read.example/" + key, ExpiresAt: time.Now().Add(ttl)}, nil
	}}
	uc, _ := NewSavedVideoUsecase(saved, storage, SavedVideoUsecaseConfig{ReadURLTTL: 10 * time.Minute})
	result, err := uc.List(context.Background(), activeVideoUser(3), VideoListInput{Limit: 2, Cursor: inputCursor})
	if err != nil {
		t.Fatalf("List() error=%v", err)
	}
	if len(result.Items) != 1 || !result.Items[0].IsSaved || !result.HasMore || result.NextCursor == nil {
		t.Fatalf("result=%+v", result)
	}
	if len(readCalls) != 2 || !strings.Contains(readCalls[0], "output") || !strings.Contains(readCalls[1], "thumbnail") {
		t.Fatalf("read calls=%v", readCalls)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*result.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor:%v", err)
	}
	var cursor VideoCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		t.Fatalf("unmarshal cursor:%v", err)
	}
	if cursor.ID != 55 || !cursor.CreatedAt.Equal(created) || cursor.CreatedAt.Location() != time.UTC {
		t.Fatalf("cursor=%+v", cursor)
	}
}

func TestSavedVideoUsecaseListReturnsNoPartialItemsWhenReadURLFails(t *testing.T) {
	saved := &savedVideoRepositoryMock{listByUserFunc: func(context.Context, uint64, int, *repository.SavedVideoCursor) (repository.SavedVideoPage, error) {
		return repository.SavedVideoPage{Items: []repository.PublicVideoItem{
			{ID: 1, VideoObjectKey: "videos/1/output/a.mp4", ThumbnailObjectKey: "videos/1/thumbnail/a.jpg"},
			{ID: 2, VideoObjectKey: "videos/2/output/b.mp4", ThumbnailObjectKey: "videos/2/thumbnail/b.jpg"},
		}}, nil
	}}
	calls := 0
	storage := &objectStorageRepositoryMock{createReadURLFunc: func(_ context.Context, key string, _ time.Duration) (repository.ReadTarget, error) {
		calls++
		if strings.Contains(key, "videos/2/output") {
			return repository.ReadTarget{}, entity.ErrStorageUnavailable
		}
		return repository.ReadTarget{URL: "https://read.example", ExpiresAt: time.Now()}, nil
	}}
	uc, _ := NewSavedVideoUsecase(saved, storage, SavedVideoUsecaseConfig{ReadURLTTL: time.Minute})
	result, err := uc.List(context.Background(), activeVideoUser(1), VideoListInput{Limit: 10})
	if !errors.Is(err, entity.ErrStorageUnavailable) {
		t.Fatalf("error=%v", err)
	}
	if len(result.Items) != 0 || result.NextCursor != nil || result.HasMore {
		t.Fatalf("partial result leaked: %+v", result)
	}
	if calls < 3 {
		t.Fatalf("read calls=%d, want failure on second item after first item conversion", calls)
	}
}

func TestSavedVideoUsecaseListRejectsInvalidActorAndPaging(t *testing.T) {
	uc, _ := NewSavedVideoUsecase(&savedVideoRepositoryMock{}, &objectStorageRepositoryMock{}, SavedVideoUsecaseConfig{ReadURLTTL: time.Minute})
	if _, err := uc.List(context.Background(), nil, VideoListInput{Limit: 10}); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("nil actor error=%v", err)
	}
	if _, err := uc.List(context.Background(), activeVideoUser(1), VideoListInput{Limit: 0}); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("limit error=%v", err)
	}
	if _, err := uc.List(context.Background(), activeVideoUser(1), VideoListInput{Limit: 10, Cursor: &VideoCursor{ID: 1}}); !errors.Is(err, entity.ErrCursorInvalid) {
		t.Fatalf("cursor error=%v", err)
	}
}

func TestSavedVideoCursorConvertsUTCAndPreservesNil(t *testing.T) {
	if got := savedVideoCursor(nil); got != nil {
		t.Fatalf("savedVideoCursor(nil)=%+v", got)
	}
	jst := time.FixedZone("JST", 9*60*60)
	input := &VideoCursor{CreatedAt: time.Date(2026, 8, 3, 12, 0, 0, 0, jst), ID: 4}
	got := savedVideoCursor(input)
	if got == nil || got.ID != 4 || got.CreatedAt.Location() != time.UTC || !got.CreatedAt.Equal(input.CreatedAt) {
		t.Fatalf("cursor=%+v", got)
	}
}
