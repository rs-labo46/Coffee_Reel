package usecase

import (
	"context"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type IVideoLikeUsecase interface {
	Like(ctx context.Context, actor *entity.User, videoID uint64) (VideoLikeResult, error)
	Unlike(ctx context.Context, actor *entity.User, videoID uint64) (VideoLikeResult, error)
}

type VideoLikeResult struct {
	VideoID   uint64
	LikeCount int64
	IsLiked   bool
}

type videoLikeUsecase struct {
	likes repository.IVideoLikeRepository
}

func NewVideoLikeUsecase(likes repository.IVideoLikeRepository) IVideoLikeUsecase {
	return &videoLikeUsecase{likes: likes}
}

func (u *videoLikeUsecase) Like(ctx context.Context, actor *entity.User, videoID uint64) (VideoLikeResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return VideoLikeResult{}, err
	}
	if videoID == 0 {
		return VideoLikeResult{}, entity.ErrInvalidInput
	}

	state, err := u.likes.Like(ctx, actor.ID, videoID, time.Now())
	if err != nil {
		return VideoLikeResult{}, err
	}
	return videoLikeResult(state), nil
}

func (u *videoLikeUsecase) Unlike(ctx context.Context, actor *entity.User, videoID uint64) (VideoLikeResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return VideoLikeResult{}, err
	}
	if videoID == 0 {
		return VideoLikeResult{}, entity.ErrInvalidInput
	}

	state, err := u.likes.Unlike(ctx, actor.ID, videoID)
	if err != nil {
		return VideoLikeResult{}, err
	}
	return videoLikeResult(state), nil
}

func videoLikeResult(state repository.VideoLikeState) VideoLikeResult {
	return VideoLikeResult{
		VideoID:   state.VideoID,
		LikeCount: state.LikeCount,
		IsLiked:   state.IsLiked,
	}
}
