//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"coffee-reel/entity"

	"gorm.io/gorm/clause"
)

func TestVideoLikeRepositoryLikeCreatesOnceAndReturnsCurrentState(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user := createSearchLikeUser(t, db, entity.StatusActive)
	other := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, user.ID, "like target", entity.CategoryBrewing, time.Now())
	if err := db.Create(&entity.VideoLike{UserID: other.ID, VideoID: video.ID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create existing other like: %v", err)
	}
	repo := NewVideoLikeRepository(db)

	state, err := repo.Like(context.Background(), user.ID, video.ID, time.Now())
	if err != nil {
		t.Fatalf("Like() error = %v", err)
	}
	if state.VideoID != video.ID || state.LikeCount != 2 || !state.IsLiked {
		t.Fatalf("state = %+v", state)
	}

	retry, err := repo.Like(context.Background(), user.ID, video.ID, time.Now())
	if err != nil {
		t.Fatalf("Like() retry error = %v", err)
	}
	if retry.LikeCount != 2 || !retry.IsLiked {
		t.Fatalf("retry state = %+v", retry)
	}
	var ownCount int64
	if err := db.Model(&entity.VideoLike{}).Where("user_id = ? AND video_id = ?", user.ID, video.ID).Count(&ownCount).Error; err != nil {
		t.Fatalf("count own like: %v", err)
	}
	if ownCount != 1 {
		t.Fatalf("own like rows = %d, want 1", ownCount)
	}
}

func TestVideoLikeRepositoryConcurrentLikeCreatesSingleRelation(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, user.ID, "concurrent target", entity.CategoryBrewing, time.Now())
	repo := NewVideoLikeRepository(db)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			state, err := repo.Like(context.Background(), user.ID, video.ID, time.Now())
			if err == nil && (state.LikeCount != 1 || !state.IsLiked) {
				err = errors.New("unexpected concurrent like state")
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Like() error = %v", err)
		}
	}
	var count int64
	if err := db.Model(&entity.VideoLike{}).Where("user_id = ? AND video_id = ?", user.ID, video.ID).Count(&count).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if count != 1 {
		t.Fatalf("like rows = %d, want 1", count)
	}
}

func TestVideoLikeRepositoryLikeRejectsInactiveOrNonPublicTarget(t *testing.T) {
	tests := []struct {
		name       string
		userStatus entity.UserStatus
		processing entity.VideoProcessingStatus
		publish    entity.VideoPublishStatus
		deleted    bool
		wantErr    error
	}{
		{name: "suspended user", userStatus: entity.StatusSuspended, processing: entity.VideoProcessingReady, publish: entity.VideoPublishPublished, wantErr: entity.ErrUserSuspended},
		{name: "private video", userStatus: entity.StatusActive, processing: entity.VideoProcessingReady, publish: entity.VideoPublishPrivate, wantErr: entity.ErrVideoNotFound},
		{name: "hidden video", userStatus: entity.StatusActive, processing: entity.VideoProcessingReady, publish: entity.VideoPublishHidden, wantErr: entity.ErrVideoNotFound},
		{name: "processing video", userStatus: entity.StatusActive, processing: entity.VideoProcessingProcessing, publish: entity.VideoPublishPrivate, wantErr: entity.ErrVideoNotFound},
		{name: "deleted video", userStatus: entity.StatusActive, processing: entity.VideoProcessingReady, publish: entity.VideoPublishPrivate, deleted: true, wantErr: entity.ErrVideoNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openSearchLikeIntegrationDB(t)
			user := createSearchLikeUser(t, db, tt.userStatus)
			video := createSearchLikeVideo(t, db, user.ID, tt.name, entity.CategoryBrewing, tt.processing, tt.publish, time.Now(), tt.deleted)
			repo := NewVideoLikeRepository(db)

			_, err := repo.Like(context.Background(), user.ID, video.ID, time.Now())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Like() error = %v, want %v", err, tt.wantErr)
			}
			var count int64
			if err := db.Model(&entity.VideoLike{}).Where("video_id = ?", video.ID).Count(&count).Error; err != nil {
				t.Fatalf("count likes: %v", err)
			}
			if count != 0 {
				t.Fatalf("like rows = %d, want 0", count)
			}
		})
	}
}

func TestVideoLikeRepositoryUnlikeDeletesOnlyActorRelationAndIsIdempotent(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user := createSearchLikeUser(t, db, entity.StatusActive)
	other := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, user.ID, "unlike target", entity.CategoryBrewing, time.Now())
	for _, userID := range []uint64{user.ID, other.ID} {
		if err := db.Create(&entity.VideoLike{UserID: userID, VideoID: video.ID, CreatedAt: time.Now()}).Error; err != nil {
			t.Fatalf("create like for user %d: %v", userID, err)
		}
	}
	repo := NewVideoLikeRepository(db)

	state, err := repo.Unlike(context.Background(), user.ID, video.ID)
	if err != nil {
		t.Fatalf("Unlike() error = %v", err)
	}
	if state.VideoID != video.ID || state.LikeCount != 1 || state.IsLiked {
		t.Fatalf("state = %+v", state)
	}
	retry, err := repo.Unlike(context.Background(), user.ID, video.ID)
	if err != nil {
		t.Fatalf("Unlike() retry error = %v", err)
	}
	if retry.LikeCount != 1 || retry.IsLiked {
		t.Fatalf("retry = %+v", retry)
	}

	var ownCount, otherCount int64
	if err := db.Model(&entity.VideoLike{}).Where("user_id = ? AND video_id = ?", user.ID, video.ID).Count(&ownCount).Error; err != nil {
		t.Fatalf("count actor like: %v", err)
	}
	if err := db.Model(&entity.VideoLike{}).Where("user_id = ? AND video_id = ?", other.ID, video.ID).Count(&otherCount).Error; err != nil {
		t.Fatalf("count other like: %v", err)
	}
	if ownCount != 0 || otherCount != 1 {
		t.Fatalf("own=%d other=%d, want 0/1", ownCount, otherCount)
	}
}

func TestVideoLikeRepositoryUnlikeRejectsHiddenVideoWithoutDeletingRelation(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, user.ID, "hidden unlike", entity.CategoryBrewing, time.Now())
	if err := db.Create(&entity.VideoLike{UserID: user.ID, VideoID: video.ID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create like: %v", err)
	}
	if err := db.Model(&entity.Video{}).Where("id = ?", video.ID).Update("publish_status", entity.VideoPublishHidden).Error; err != nil {
		t.Fatalf("hide video: %v", err)
	}
	repo := NewVideoLikeRepository(db)

	_, err := repo.Unlike(context.Background(), user.ID, video.ID)
	if !errors.Is(err, entity.ErrVideoNotFound) {
		t.Fatalf("Unlike() error = %v, want ErrVideoNotFound", err)
	}
	var count int64
	if err := db.Model(&entity.VideoLike{}).Where("user_id = ? AND video_id = ?", user.ID, video.ID).Count(&count).Error; err != nil {
		t.Fatalf("count retained like: %v", err)
	}
	if count != 1 {
		t.Fatalf("retained like rows = %d, want 1", count)
	}

	if err := db.Model(&entity.Video{}).Where("id = ?", video.ID).Update("publish_status", entity.VideoPublishPublished).Error; err != nil {
		t.Fatalf("restore video: %v", err)
	}
	videoRepo := NewVideoRepository(db)
	page, err := videoRepo.ListPublic(context.Background(), PublicVideoListInput{Limit: 20, SearchMode: PublicVideoSearchAll})
	if err != nil {
		t.Fatalf("ListPublic() after restore error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != video.ID || page.Items[0].LikeCount != 1 {
		t.Fatalf("restored video state = %+v, want retained like_count=1", page.Items)
	}
}

func TestVideoLikeRepositoryLikeWaitsForAdminHideAndRejectsAfterHiddenCommit(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, user.ID, "hide race", entity.CategoryBrewing, time.Now())
	repo := NewVideoLikeRepository(db)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin hide transaction: %v", tx.Error)
	}
	var locked entity.Video
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", video.ID).Take(&locked).Error; err != nil {
		t.Fatalf("lock video: %v", err)
	}
	if err := tx.Model(&entity.Video{}).Where("id = ?", video.ID).Update("publish_status", entity.VideoPublishHidden).Error; err != nil {
		t.Fatalf("hide video in transaction: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := repo.Like(context.Background(), user.ID, video.ID, time.Now())
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Like() finished before hide transaction committed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit hide transaction: %v", err)
	}
	if err := <-done; !errors.Is(err, entity.ErrVideoNotFound) {
		t.Fatalf("Like() after hidden commit error = %v", err)
	}
	var count int64
	if err := db.Model(&entity.VideoLike{}).Where("video_id = ?", video.ID).Count(&count).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if count != 0 {
		t.Fatalf("like rows = %d, want 0", count)
	}
}

func TestVideoLikeRepositoryLikeWaitsForUserSuspendAndRejectsAfterCommit(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	user := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, user.ID, "suspend race", entity.CategoryBrewing, time.Now())
	repo := NewVideoLikeRepository(db)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin suspend transaction: %v", tx.Error)
	}
	var locked entity.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).Take(&locked).Error; err != nil {
		t.Fatalf("lock user: %v", err)
	}
	if err := tx.Model(&entity.User{}).Where("id = ?", user.ID).Update("status", entity.StatusSuspended).Error; err != nil {
		t.Fatalf("suspend user in transaction: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := repo.Like(context.Background(), user.ID, video.ID, time.Now())
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Like() finished before suspend transaction committed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit suspend transaction: %v", err)
	}
	if err := <-done; !errors.Is(err, entity.ErrUserSuspended) {
		t.Fatalf("Like() after suspended commit error = %v", err)
	}
}
