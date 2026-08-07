package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type videoLikeRepositoryMock struct {
	likeFunc   func(context.Context, uint64, uint64, time.Time) (repository.VideoLikeState, error)
	unlikeFunc func(context.Context, uint64, uint64) (repository.VideoLikeState, error)
}

func (m *videoLikeRepositoryMock) Like(ctx context.Context, userID, videoID uint64, now time.Time) (repository.VideoLikeState, error) {
	if m.likeFunc == nil {
		panic("unexpected VideoLikeRepository.Like call")
	}
	return m.likeFunc(ctx, userID, videoID, now)
}

func (m *videoLikeRepositoryMock) Unlike(ctx context.Context, userID, videoID uint64) (repository.VideoLikeState, error) {
	if m.unlikeFunc == nil {
		panic("unexpected VideoLikeRepository.Unlike call")
	}
	return m.unlikeFunc(ctx, userID, videoID)
}

func TestVideoLikeUsecaseLikeReturnsRepositoryState(t *testing.T) {
	before := time.Now()
	likes := &videoLikeRepositoryMock{likeFunc: func(_ context.Context, userID, videoID uint64, now time.Time) (repository.VideoLikeState, error) {
		if userID != 9 || videoID != 12 {
			t.Fatalf("Like(userID=%d, videoID=%d)", userID, videoID)
		}
		if now.Before(before) || now.After(time.Now()) {
			t.Fatalf("now = %s, want current time", now)
		}
		return repository.VideoLikeState{VideoID: videoID, LikeCount: 4, IsLiked: true}, nil
	}}
	uc := NewVideoLikeUsecase(likes)

	result, err := uc.Like(context.Background(), activeVideoUser(9), 12)
	if err != nil {
		t.Fatalf("Like() error = %v", err)
	}
	if result.VideoID != 12 || result.LikeCount != 4 || !result.IsLiked {
		t.Fatalf("result = %+v", result)
	}
}

func TestVideoLikeUsecaseLikeDuplicateReturnsCurrentState(t *testing.T) {
	likes := &videoLikeRepositoryMock{likeFunc: func(_ context.Context, userID, videoID uint64, _ time.Time) (repository.VideoLikeState, error) {
		return repository.VideoLikeState{VideoID: videoID, LikeCount: 7, IsLiked: true}, nil
	}}
	uc := NewVideoLikeUsecase(likes)

	result, err := uc.Like(context.Background(), activeVideoUser(3), 8)
	if err != nil {
		t.Fatalf("Like() error = %v", err)
	}
	if result.VideoID != 8 || result.LikeCount != 7 || !result.IsLiked {
		t.Fatalf("result = %+v", result)
	}
}

func TestVideoLikeUsecaseUnlikeReturnsRepositoryState(t *testing.T) {
	likes := &videoLikeRepositoryMock{unlikeFunc: func(_ context.Context, userID, videoID uint64) (repository.VideoLikeState, error) {
		if userID != 5 || videoID != 14 {
			t.Fatalf("Unlike(userID=%d, videoID=%d)", userID, videoID)
		}
		return repository.VideoLikeState{VideoID: videoID, LikeCount: 2, IsLiked: false}, nil
	}}
	uc := NewVideoLikeUsecase(likes)

	result, err := uc.Unlike(context.Background(), activeVideoUser(5), 14)
	if err != nil {
		t.Fatalf("Unlike() error = %v", err)
	}
	if result.VideoID != 14 || result.LikeCount != 2 || result.IsLiked {
		t.Fatalf("result = %+v", result)
	}
}

func TestVideoLikeUsecaseUnlikeRetryReturnsCurrentState(t *testing.T) {
	likes := &videoLikeRepositoryMock{unlikeFunc: func(_ context.Context, _ uint64, videoID uint64) (repository.VideoLikeState, error) {
		return repository.VideoLikeState{VideoID: videoID, LikeCount: 2, IsLiked: false}, nil
	}}
	uc := NewVideoLikeUsecase(likes)

	result, err := uc.Unlike(context.Background(), activeVideoUser(5), 14)
	if err != nil {
		t.Fatalf("Unlike() error = %v", err)
	}
	if result.LikeCount != 2 || result.IsLiked {
		t.Fatalf("result = %+v", result)
	}
}

func TestVideoLikeUsecaseRejectsInvalidActorAndVideoID(t *testing.T) {
	called := false
	likes := &videoLikeRepositoryMock{
		likeFunc: func(context.Context, uint64, uint64, time.Time) (repository.VideoLikeState, error) {
			called = true
			return repository.VideoLikeState{}, nil
		},
		unlikeFunc: func(context.Context, uint64, uint64) (repository.VideoLikeState, error) {
			called = true
			return repository.VideoLikeState{}, nil
		},
	}
	uc := NewVideoLikeUsecase(likes)

	if _, err := uc.Like(context.Background(), nil, 1); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("Like(nil) error = %v", err)
	}
	if _, err := uc.Like(context.Background(), suspendedVideoUser(1), 1); !errors.Is(err, entity.ErrUserSuspended) {
		t.Fatalf("Like(suspended) error = %v", err)
	}
	if _, err := uc.Like(context.Background(), activeVideoUser(1), 0); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("Like(videoID=0) error = %v", err)
	}
	if _, err := uc.Unlike(context.Background(), activeVideoUser(1), 0); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("Unlike(videoID=0) error = %v", err)
	}
	if called {
		t.Fatal("repository was called for invalid input")
	}
}

func TestVideoLikeUsecasePropagatesNonPublicVideoError(t *testing.T) {
	likes := &videoLikeRepositoryMock{
		likeFunc: func(context.Context, uint64, uint64, time.Time) (repository.VideoLikeState, error) {
			return repository.VideoLikeState{}, entity.ErrVideoNotFound
		},
		unlikeFunc: func(context.Context, uint64, uint64) (repository.VideoLikeState, error) {
			return repository.VideoLikeState{}, entity.ErrVideoNotFound
		},
	}
	uc := NewVideoLikeUsecase(likes)

	if _, err := uc.Like(context.Background(), activeVideoUser(1), 2); !errors.Is(err, entity.ErrVideoNotFound) {
		t.Fatalf("Like() error = %v", err)
	}
	if _, err := uc.Unlike(context.Background(), activeVideoUser(1), 2); !errors.Is(err, entity.ErrVideoNotFound) {
		t.Fatalf("Unlike() error = %v", err)
	}
}
