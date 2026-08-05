// ---追加---
package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type IAdminVideoUsecase interface {
	List(
		ctx context.Context,
		actor *entity.User,
		input AdminVideoListInput,
	) (AdminVideoListResult, error)

	Get(
		ctx context.Context,
		actor *entity.User,
		videoID uint64,
	) (AdminVideoDetailResult, error)

	Hide(
		ctx context.Context,
		actor *entity.User,
		videoID uint64,
		reason string,
		requestID string,
	) (AdminVideoStateResult, error)

	Restore(
		ctx context.Context,
		actor *entity.User,
		videoID uint64,
		reason string,
		requestID string,
	) (AdminVideoStateResult, error)
}

type AdminVideoUsecaseConfig struct {
	ReadURLTTL time.Duration
}

type AdminVideoCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uint64    `json:"id"`
}

type AdminVideoListInput struct {
	Limit  int
	Cursor *AdminVideoCursor
}

type AdminVideoAuthorResult struct {
	ID     uint64
	Name   string
	Status entity.UserStatus
}

type AdminVideoListItem struct {
	ID               uint64
	Author           AdminVideoAuthorResult
	Title            string
	Description      string
	Category         entity.CategoryCode
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AdminVideoListResult struct {
	Items      []AdminVideoListItem
	NextCursor *string
	HasMore    bool
}

type AdminVideoDetailResult struct {
	ID               uint64
	Author           AdminVideoAuthorResult
	Title            string
	Description      string
	Category         entity.CategoryCode
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	PlaybackURL      *string
	ThumbnailURL     *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AdminVideoStateResult struct {
	ID               uint64
	ProcessingStatus entity.VideoProcessingStatus
	PublishStatus    entity.VideoPublishStatus
	UpdatedAt        time.Time
}

type adminVideoUsecase struct {
	videos     repository.IAdminVideoRepository
	storage    repository.IObjectStorageRepository
	readURLTTL time.Duration
}

func NewAdminVideoUsecase(videos repository.IAdminVideoRepository, storage repository.IObjectStorageRepository, config AdminVideoUsecaseConfig) (IAdminVideoUsecase, error) {
	if videos == nil ||
		storage == nil ||
		config.ReadURLTTL <= 0 {
		return nil, fmt.Errorf(
			"admin video usecase configuration is invalid",
		)
	}

	return &adminVideoUsecase{
		videos:     videos,
		storage:    storage,
		readURLTTL: config.ReadURLTTL,
	}, nil
}

func (u *adminVideoUsecase) List(ctx context.Context, actor *entity.User, input AdminVideoListInput) (AdminVideoListResult, error) {
	if err := validateAdminActor(actor); err != nil {
		return AdminVideoListResult{}, err
	}

	if err := validateAdminVideoListInput(input); err != nil {
		return AdminVideoListResult{}, err
	}

	page, err := u.videos.List(
		ctx,
		input.Limit,
		repositoryAdminVideoCursor(input.Cursor),
	)
	if err != nil {
		return AdminVideoListResult{}, err
	}

	items := make([]AdminVideoListItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, AdminVideoListItem{
			ID: item.VideoID,
			Author: AdminVideoAuthorResult{
				ID:     item.AuthorID,
				Name:   item.AuthorName,
				Status: item.AuthorStatus,
			},
			Title:            item.Title,
			Description:      item.Description,
			Category:         item.Category,
			ProcessingStatus: item.ProcessingStatus,
			PublishStatus:    item.PublishStatus,
			CreatedAt:        item.CreatedAt.UTC(),
			UpdatedAt:        item.UpdatedAt.UTC(),
		})
	}

	result := AdminVideoListResult{
		Items:   items,
		HasMore: page.HasMore,
	}

	if page.HasMore {
		if len(items) == 0 {
			return AdminVideoListResult{}, fmt.Errorf(
				"admin video list cursor source is missing",
			)
		}

		last := items[len(items)-1]

		cursor, err := encodeAdminVideoCursor(
			AdminVideoCursor{
				CreatedAt: last.CreatedAt.UTC(),
				ID:        last.ID,
			},
		)
		if err != nil {
			return AdminVideoListResult{}, err
		}

		result.NextCursor = &cursor
	}

	return result, nil
}

func (u *adminVideoUsecase) Get(ctx context.Context, actor *entity.User, videoID uint64) (AdminVideoDetailResult, error) {
	if err := validateAdminActor(actor); err != nil {
		return AdminVideoDetailResult{}, err
	}

	if videoID == 0 || videoID > math.MaxInt64 {
		return AdminVideoDetailResult{}, entity.ErrInvalidInput
	}

	detail, err := u.videos.FindByID(ctx, videoID)
	if err != nil {
		return AdminVideoDetailResult{}, err
	}

	if detail == nil {
		return AdminVideoDetailResult{}, entity.ErrVideoNotFound
	}

	result := AdminVideoDetailResult{
		ID: detail.VideoID,
		Author: AdminVideoAuthorResult{
			ID:     detail.AuthorID,
			Name:   detail.AuthorName,
			Status: detail.AuthorStatus,
		},
		Title:            detail.Title,
		Description:      detail.Description,
		Category:         detail.Category,
		ProcessingStatus: detail.ProcessingStatus,
		PublishStatus:    detail.PublishStatus,
		CreatedAt:        detail.CreatedAt.UTC(),
		UpdatedAt:        detail.UpdatedAt.UTC(),
	}

	if !canIssueAdminVideoReadURL(detail) {
		return result, nil
	}

	if detail.OutputMeta == nil ||
		strings.TrimSpace(
			detail.OutputMeta.VideoObjectKey,
		) == "" ||
		strings.TrimSpace(
			detail.OutputMeta.ThumbnailObjectKey,
		) == "" {
		return AdminVideoDetailResult{}, fmt.Errorf(
			"admin video output metadata is missing",
		)
	}

	playbackTarget, err := u.storage.CreateReadURL(
		ctx,
		detail.OutputMeta.VideoObjectKey,
		u.readURLTTL,
	)
	if err != nil {
		return AdminVideoDetailResult{}, fmt.Errorf(
			"create admin video playback URL: %w",
			err,
		)
	}

	thumbnailTarget, err := u.storage.CreateReadURL(
		ctx,
		detail.OutputMeta.ThumbnailObjectKey,
		u.readURLTTL,
	)
	if err != nil {
		return AdminVideoDetailResult{}, fmt.Errorf(
			"create admin video thumbnail URL: %w",
			err,
		)
	}

	playbackURL := playbackTarget.URL
	thumbnailURL := thumbnailTarget.URL

	result.PlaybackURL = &playbackURL
	result.ThumbnailURL = &thumbnailURL

	return result, nil
}

func (u *adminVideoUsecase) Hide(ctx context.Context, actor *entity.User, videoID uint64, reason string, requestID string) (AdminVideoStateResult, error) {
	reason = strings.TrimSpace(reason)
	requestID = strings.TrimSpace(requestID)

	if err := validateAdminVideoOperation(
		actor,
		videoID,
		reason,
		requestID,
	); err != nil {
		return AdminVideoStateResult{}, err
	}

	state, err := u.videos.Hide(
		ctx,
		actor.ID,
		videoID,
		reason,
		requestID,
		time.Now().UTC(),
	)
	if err != nil {
		return AdminVideoStateResult{}, err
	}

	if state == nil {
		return AdminVideoStateResult{}, fmt.Errorf(
			"admin video hide returned no state",
		)
	}

	return adminVideoStateResult(state), nil
}

func (u *adminVideoUsecase) Restore(ctx context.Context, actor *entity.User, videoID uint64, reason string, requestID string) (AdminVideoStateResult, error) {
	reason = strings.TrimSpace(reason)
	requestID = strings.TrimSpace(requestID)

	if err := validateAdminVideoOperation(
		actor,
		videoID,
		reason,
		requestID,
	); err != nil {
		return AdminVideoStateResult{}, err
	}

	state, err := u.videos.Restore(
		ctx,
		actor.ID,
		videoID,
		reason,
		requestID,
		time.Now().UTC(),
	)
	if err != nil {
		return AdminVideoStateResult{}, err
	}

	if state == nil {
		return AdminVideoStateResult{}, fmt.Errorf(
			"admin video restore returned no state",
		)
	}

	return adminVideoStateResult(state), nil
}

func validateAdminVideoListInput(input AdminVideoListInput) error {
	if input.Limit < 1 || input.Limit > 100 {
		return entity.ErrInvalidInput
	}

	if input.Cursor != nil &&
		(input.Cursor.CreatedAt.IsZero() ||
			input.Cursor.ID == 0 ||
			input.Cursor.ID > math.MaxInt64) {
		return entity.ErrCursorInvalid
	}

	return nil
}

func validateAdminVideoOperation(actor *entity.User, videoID uint64, reason string, requestID string) error {
	if err := validateAdminActor(actor); err != nil {
		return err
	}

	if videoID == 0 ||
		videoID > math.MaxInt64 ||
		reason == "" ||
		requestID == "" {
		return entity.ErrInvalidInput
	}

	return nil
}

func repositoryAdminVideoCursor(cursor *AdminVideoCursor) *repository.AdminVideoCursor {
	if cursor == nil {
		return nil
	}

	return &repository.AdminVideoCursor{
		CreatedAt: cursor.CreatedAt.UTC(),
		ID:        cursor.ID,
	}
}

func encodeAdminVideoCursor(cursor AdminVideoCursor) (string, error) {
	payload, err := json.Marshal(AdminVideoCursor{
		CreatedAt: cursor.CreatedAt.UTC(),
		ID:        cursor.ID,
	})
	if err != nil {
		return "", fmt.Errorf(
			"encode admin video cursor: %w",
			err,
		)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func canIssueAdminVideoReadURL(detail *repository.AdminVideoDetail) bool {
	if detail == nil ||
		detail.ProcessingStatus != entity.VideoProcessingReady {
		return false
	}

	return detail.PublishStatus == entity.VideoPublishPublished ||
		detail.PublishStatus == entity.VideoPublishHidden
}

func adminVideoStateResult(state *repository.AdminVideoState) AdminVideoStateResult {
	return AdminVideoStateResult{
		ID:               state.VideoID,
		ProcessingStatus: state.ProcessingStatus,
		PublishStatus:    state.PublishStatus,
		UpdatedAt:        state.UpdatedAt.UTC(),
	}
}
