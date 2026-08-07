package usecase

import (
	"context"
	"fmt"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type SavedVideoUsecaseConfig struct {
	ReadURLTTL time.Duration
}

type ISavedVideoUsecase interface {
	Save(ctx context.Context, actor *entity.User, videoID uint64) error
	Remove(ctx context.Context, actor *entity.User, videoID uint64) error
	List(ctx context.Context, actor *entity.User, input VideoListInput) (PublicVideoListResult, error)
}

type savedVideoUsecase struct {
	saved      repository.ISavedVideoRepository
	storage    repository.IObjectStorageRepository
	readURLTTL time.Duration
}

func NewSavedVideoUsecase(saved repository.ISavedVideoRepository, storage repository.IObjectStorageRepository, config SavedVideoUsecaseConfig) (ISavedVideoUsecase, error) {
	if saved == nil || storage == nil || config.ReadURLTTL <= 0 {
		return nil, fmt.Errorf("saved video usecase configuration is invalid")
	}

	return &savedVideoUsecase{
		saved:      saved,
		storage:    storage,
		readURLTTL: config.ReadURLTTL,
	}, nil
}

func (u *savedVideoUsecase) Save(ctx context.Context, actor *entity.User, videoID uint64) error {
	if err := validateActiveActor(actor); err != nil {
		return err
	}
	if videoID == 0 {
		return entity.ErrInvalidInput
	}

	return u.saved.Save(ctx, actor.ID, videoID, time.Now())
}

func (u *savedVideoUsecase) Remove(ctx context.Context, actor *entity.User, videoID uint64) error {
	if err := validateActiveActor(actor); err != nil {
		return err
	}
	if videoID == 0 {
		return entity.ErrInvalidInput
	}

	return u.saved.Remove(ctx, actor.ID, videoID)
}

func (u *savedVideoUsecase) List(ctx context.Context, actor *entity.User, input VideoListInput) (PublicVideoListResult, error) {
	if err := validateActiveActor(actor); err != nil {
		return PublicVideoListResult{}, err
	}
	if err := validateVideoListInput(input); err != nil {
		return PublicVideoListResult{}, err
	}

	page, err := u.saved.ListByUser(ctx, actor.ID, input.Limit, savedVideoCursor(input.Cursor))
	if err != nil {
		return PublicVideoListResult{}, err
	}

	items := make([]PublicVideoResult, 0, len(page.Items))
	for _, item := range page.Items {
		result, err := buildPublicVideoResult(ctx, u.storage, u.readURLTTL, item)
		if err != nil {
			return PublicVideoListResult{}, err
		}
		items = append(items, result)
	}

	output := PublicVideoListResult{
		Items:   items,
		HasMore: page.HasMore,
	}
	if page.HasMore && len(items) > 0 {
		cursor, err := encodeVideoCursor(VideoCursor{
			CreatedAt: page.LastCreatedAt,
			ID:        page.LastID,
		})
		if err != nil {
			return PublicVideoListResult{}, err
		}
		output.NextCursor = &cursor
	}

	return output, nil
}

func savedVideoCursor(cursor *VideoCursor) *repository.SavedVideoCursor {
	if cursor == nil {
		return nil
	}

	return &repository.SavedVideoCursor{
		CreatedAt: cursor.CreatedAt,
		ID:        cursor.ID,
	}
}
