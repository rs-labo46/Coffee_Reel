//go:build integration

package repository

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"coffee-reel/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVideoRepositoryListPublicAllUsesPublicConditionsAndStableOrder(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Date(2026, 8, 8, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))

	old := createPublicSearchLikeVideo(t, db, author.ID, "old public", entity.CategoryBrewing, base)
	new1 := createPublicSearchLikeVideo(t, db, author.ID, "new public 1", entity.CategoryBrewing, base.Add(time.Hour))
	new2 := createPublicSearchLikeVideo(t, db, author.ID, "new public 2", entity.CategoryBrewing, base.Add(time.Hour))

	hidden := createSearchLikeVideo(t, db, author.ID, "hidden", entity.CategoryBrewing, entity.VideoProcessingReady, entity.VideoPublishHidden, base.Add(2*time.Hour), false)
	createSearchLikeOutput(t, db, hidden.ID, hidden.CreatedAt)
	private := createSearchLikeVideo(t, db, author.ID, "private", entity.CategoryBrewing, entity.VideoProcessingReady, entity.VideoPublishPrivate, base.Add(3*time.Hour), false)
	createSearchLikeOutput(t, db, private.ID, private.CreatedAt)
	processing := createSearchLikeVideo(t, db, author.ID, "processing", entity.CategoryBrewing, entity.VideoProcessingProcessing, entity.VideoPublishPrivate, base.Add(4*time.Hour), false)
	createSearchLikeOutput(t, db, processing.ID, processing.CreatedAt)
	failed := createSearchLikeVideo(t, db, author.ID, "failed", entity.CategoryBrewing, entity.VideoProcessingFailed, entity.VideoPublishPrivate, base.Add(5*time.Hour), false)
	createSearchLikeOutput(t, db, failed.ID, failed.CreatedAt)
	deleted := createSearchLikeVideo(t, db, author.ID, "deleted", entity.CategoryBrewing, entity.VideoProcessingReady, entity.VideoPublishPrivate, base.Add(6*time.Hour), true)
	createSearchLikeOutput(t, db, deleted.ID, deleted.CreatedAt)

	repo := NewVideoRepository(db)
	page, err := repo.ListPublic(context.Background(), PublicVideoListInput{Limit: 20, SearchMode: PublicVideoSearchAll})
	if err != nil {
		t.Fatalf("ListPublic() error = %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("items = %+v, want 3 public videos", page.Items)
	}
	if page.Items[0].ID != new2.ID || page.Items[1].ID != new1.ID || page.Items[2].ID != old.ID {
		t.Fatalf("order = [%d %d %d], want [%d %d %d]", page.Items[0].ID, page.Items[1].ID, page.Items[2].ID, new2.ID, new1.ID, old.ID)
	}
}

func TestVideoRepositoryListPublicMatchedSearch(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Now()
	latteBrewing := createPublicSearchLikeVideo(t, db, author.ID, "Morning LATTE Art", entity.CategoryBrewing, base)
	latteBeans := createPublicSearchLikeVideo(t, db, author.ID, "Latte Beans", entity.CategoryBeans, base.Add(time.Second))
	handDrip := createPublicSearchLikeVideo(t, db, author.ID, "Hand Drip", entity.CategoryBrewing, base.Add(2*time.Second))
	repo := NewVideoRepository(db)

	tests := []struct {
		name     string
		title    string
		category *entity.CategoryCode
		wantIDs  []uint64
	}{
		{name: "title exact case insensitive", title: "morning latte art", wantIDs: []uint64{latteBrewing.ID}},
		{name: "title partial case insensitive", title: "latte", wantIDs: []uint64{latteBeans.ID, latteBrewing.ID}},
		{name: "category exact", category: categoryPtr(entity.CategoryBrewing), wantIDs: []uint64{handDrip.ID, latteBrewing.ID}},
		{name: "title and category", title: "latte", category: categoryPtr(entity.CategoryBeans), wantIDs: []uint64{latteBeans.ID}},
		{name: "title match category mismatch", title: "morning", category: categoryPtr(entity.CategoryBeans), wantIDs: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := repo.ListPublic(context.Background(), PublicVideoListInput{
				Title: tt.title, Category: tt.category, Limit: 20, SearchMode: PublicVideoSearchMatched,
			})
			if err != nil {
				t.Fatalf("ListPublic() error = %v", err)
			}
			if len(page.Items) != len(tt.wantIDs) {
				t.Fatalf("items = %+v, want IDs %v", page.Items, tt.wantIDs)
			}
			for i, wantID := range tt.wantIDs {
				if page.Items[i].ID != wantID {
					t.Fatalf("item[%d].ID = %d, want %d", i, page.Items[i].ID, wantID)
				}
			}
		})
	}
}

func TestVideoRepositoryListPublicTreatsSearchMetacharactersAsLiterals(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Now()
	percent := createPublicSearchLikeVideo(t, db, author.ID, "100% coffee", entity.CategoryBrewing, base)
	underscore := createPublicSearchLikeVideo(t, db, author.ID, "under_score", entity.CategoryBrewing, base.Add(time.Second))
	backslash := createPublicSearchLikeVideo(t, db, author.ID, `back\slash`, entity.CategoryBrewing, base.Add(2*time.Second))
	createPublicSearchLikeVideo(t, db, author.ID, "ordinary title", entity.CategoryBrewing, base.Add(3*time.Second))
	repo := NewVideoRepository(db)

	for _, tt := range []struct {
		query  string
		wantID uint64
	}{
		{query: "%", wantID: percent.ID},
		{query: "_", wantID: underscore.ID},
		{query: `\`, wantID: backslash.ID},
	} {
		page, err := repo.ListPublic(context.Background(), PublicVideoListInput{Title: tt.query, Limit: 20, SearchMode: PublicVideoSearchMatched})
		if err != nil {
			t.Fatalf("query %q: %v", tt.query, err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != tt.wantID {
			t.Fatalf("query %q items = %+v, want video %d", tt.query, page.Items, tt.wantID)
		}
	}

	injection := `' OR 1=1 --`
	page, err := repo.ListPublic(context.Background(), PublicVideoListInput{Title: injection, Limit: 20, SearchMode: PublicVideoSearchMatched})
	if err != nil {
		t.Fatalf("SQL injection-shaped query error = %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("SQL injection-shaped query returned %+v", page.Items)
	}
	var count int64
	if err := db.Model(&entity.Video{}).Count(&count).Error; err != nil {
		t.Fatalf("count videos: %v", err)
	}
	if count != 4 {
		t.Fatalf("video count = %d, want 4", count)
	}
}

func TestVideoRepositoryListPublicLimitPlusOneAndCursorHaveNoDuplicateOrGap(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Now()
	v1 := createPublicSearchLikeVideo(t, db, author.ID, "one", entity.CategoryBrewing, base)
	v2 := createPublicSearchLikeVideo(t, db, author.ID, "two", entity.CategoryBrewing, base.Add(time.Second))
	v3 := createPublicSearchLikeVideo(t, db, author.ID, "three", entity.CategoryBrewing, base.Add(2*time.Second))
	repo := NewVideoRepository(db)

	first, err := repo.ListPublic(context.Background(), PublicVideoListInput{Limit: 2, SearchMode: PublicVideoSearchAll})
	if err != nil {
		t.Fatalf("first ListPublic() error = %v", err)
	}
	if !first.HasMore || len(first.Items) != 2 || first.Items[0].ID != v3.ID || first.Items[1].ID != v2.ID {
		t.Fatalf("first page = %+v", first)
	}
	second, err := repo.ListPublic(context.Background(), PublicVideoListInput{
		Limit:      2,
		SearchMode: PublicVideoSearchAll,
		Cursor: &PublicVideoCursor{
			ResultType: PublicVideoSearchAll,
			CreatedAt:  first.LastCreatedAt,
			ID:         first.LastID,
			FilterHash: strings.Repeat("a", 64),
		},
	})
	if err != nil {
		t.Fatalf("second ListPublic() error = %v", err)
	}
	if second.HasMore || len(second.Items) != 1 || second.Items[0].ID != v1.ID {
		t.Fatalf("second page = %+v", second)
	}
}

func TestVideoRepositoryListPublicSimilarUsesSimilarityOrderAndPublicConditions(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Now()
	createPublicSearchLikeVideo(t, db, author.ID, "latte art", entity.CategoryLatteArt, base)
	createPublicSearchLikeVideo(t, db, author.ID, "latte art basics", entity.CategoryLatteArt, base.Add(time.Second))
	hidden := createSearchLikeVideo(t, db, author.ID, "latte art hidden", entity.CategoryLatteArt, entity.VideoProcessingReady, entity.VideoPublishHidden, base.Add(2*time.Second), false)
	createSearchLikeOutput(t, db, hidden.ID, hidden.CreatedAt)
	repo := NewVideoRepository(db)

	page, err := repo.ListPublic(context.Background(), PublicVideoListInput{Title: "latte art", Limit: 20, SearchMode: PublicVideoSearchSimilar})
	if err != nil {
		t.Fatalf("ListPublic(similar) error = %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("similar search returned no candidates for exact/near titles")
	}
	for i, item := range page.Items {
		if item.ID == hidden.ID {
			t.Fatal("hidden video was returned by similar search")
		}
		if item.Similarity < 0.6 || item.Similarity > 1 {
			t.Fatalf("item[%d] similarity = %f", i, item.Similarity)
		}
		if i > 0 && page.Items[i-1].Similarity < item.Similarity {
			t.Fatalf("similarity order = %+v", page.Items)
		}
	}
}

func TestVideoRepositoryListPublicSimilarCursorHasNoDuplicateOrGap(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Now()
	v1 := createPublicSearchLikeVideo(t, db, author.ID, "latte art one", entity.CategoryLatteArt, base)
	v2 := createPublicSearchLikeVideo(t, db, author.ID, "latte art two", entity.CategoryLatteArt, base.Add(time.Second))
	v3 := createPublicSearchLikeVideo(t, db, author.ID, "latte art three", entity.CategoryLatteArt, base.Add(2*time.Second))
	repo := NewVideoRepository(db)

	first, err := repo.ListPublic(context.Background(), PublicVideoListInput{
		Title:      "latte art",
		Limit:      2,
		SearchMode: PublicVideoSearchSimilar,
	})
	if err != nil {
		t.Fatalf("first ListPublic(similar) error = %v", err)
	}
	if !first.HasMore || len(first.Items) != 2 || first.Items[0].ID != v3.ID || first.Items[1].ID != v2.ID {
		t.Fatalf("first similar page = %+v", first)
	}

	second, err := repo.ListPublic(context.Background(), PublicVideoListInput{
		Title:      "latte art",
		Limit:      2,
		SearchMode: PublicVideoSearchSimilar,
		Cursor: &PublicVideoCursor{
			ResultType: PublicVideoSearchSimilar,
			Similarity: first.LastSimilarity,
			CreatedAt:  first.LastCreatedAt,
			ID:         first.LastID,
			FilterHash: strings.Repeat("b", 64),
		},
	})
	if err != nil {
		t.Fatalf("second ListPublic(similar) error = %v", err)
	}
	if second.HasMore || len(second.Items) != 1 || second.Items[0].ID != v1.ID {
		t.Fatalf("second similar page = %+v", second)
	}
}

func TestVideoRepositoryPublicStateForGuestAndViewer(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	viewer := createSearchLikeUser(t, db, entity.StatusActive)
	other := createSearchLikeUser(t, db, entity.StatusActive)
	video := createPublicSearchLikeVideo(t, db, author.ID, "state video", entity.CategoryBrewing, time.Now())
	if err := db.Create(&entity.VideoLike{UserID: viewer.ID, VideoID: video.ID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create viewer like: %v", err)
	}
	if err := db.Create(&entity.VideoLike{UserID: other.ID, VideoID: video.ID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create other like: %v", err)
	}
	if err := db.Create(&entity.SavedVideo{UserID: viewer.ID, VideoID: video.ID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create saved video: %v", err)
	}
	repo := NewVideoRepository(db)

	guest, err := repo.ListPublic(context.Background(), PublicVideoListInput{Limit: 20, SearchMode: PublicVideoSearchAll})
	if err != nil {
		t.Fatalf("guest ListPublic() error = %v", err)
	}
	if len(guest.Items) != 1 || guest.Items[0].LikeCount != 2 || guest.Items[0].IsLiked || guest.Items[0].IsSaved {
		t.Fatalf("guest item = %+v", guest.Items)
	}
	viewerID := viewer.ID
	loggedIn, err := repo.ListPublic(context.Background(), PublicVideoListInput{Limit: 20, SearchMode: PublicVideoSearchAll, ViewerUserID: &viewerID})
	if err != nil {
		t.Fatalf("viewer ListPublic() error = %v", err)
	}
	if len(loggedIn.Items) != 1 || loggedIn.Items[0].LikeCount != 2 || !loggedIn.Items[0].IsLiked || !loggedIn.Items[0].IsSaved {
		t.Fatalf("viewer item = %+v", loggedIn.Items)
	}

	guestDetail, err := repo.FindPublicByID(context.Background(), video.ID, nil)
	if err != nil {
		t.Fatalf("guest FindPublicByID() error = %v", err)
	}
	if guestDetail == nil || guestDetail.LikeCount != 2 || guestDetail.IsLiked || guestDetail.IsSaved {
		t.Fatalf("guest detail = %+v", guestDetail)
	}

	viewerDetail, err := repo.FindPublicByID(context.Background(), video.ID, &viewerID)
	if err != nil {
		t.Fatalf("viewer FindPublicByID() error = %v", err)
	}
	if viewerDetail == nil || viewerDetail.LikeCount != 2 || !viewerDetail.IsLiked || !viewerDetail.IsSaved {
		t.Fatalf("viewer detail = %+v", viewerDetail)
	}
}

type queryCountingLogger struct {
	logger.Interface
	count atomic.Int64
}

func (l *queryCountingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.count.Add(1)
	l.Interface.Trace(ctx, begin, fc, err)
}

func TestVideoRepositoryListPublicQueryCountDoesNotScaleWithVideoCount(t *testing.T) {
	db := openSearchLikeIntegrationDB(t)
	author := createSearchLikeUser(t, db, entity.StatusActive)
	viewer := createSearchLikeUser(t, db, entity.StatusActive)
	base := time.Now()
	for i := 0; i < 5; i++ {
		video := createPublicSearchLikeVideo(t, db, author.ID, "query count", entity.CategoryBrewing, base.Add(time.Duration(i)*time.Second))
		if err := db.Create(&entity.VideoLike{UserID: viewer.ID, VideoID: video.ID, CreatedAt: base}).Error; err != nil {
			t.Fatalf("create like: %v", err)
		}
		if err := db.Create(&entity.SavedVideo{UserID: viewer.ID, VideoID: video.ID, CreatedAt: base}).Error; err != nil {
			t.Fatalf("create saved: %v", err)
		}
	}

	counter := &queryCountingLogger{Interface: db.Logger}
	countedDB := db.Session(&gorm.Session{Logger: counter})
	repo := NewVideoRepository(countedDB)
	viewerID := viewer.ID
	page, err := repo.ListPublic(context.Background(), PublicVideoListInput{Limit: 20, SearchMode: PublicVideoSearchAll, ViewerUserID: &viewerID})
	if err != nil {
		t.Fatalf("ListPublic() error = %v", err)
	}
	if len(page.Items) != 5 {
		t.Fatalf("items = %d, want 5", len(page.Items))
	}
	if got := counter.count.Load(); got != 1 {
		t.Fatalf("ListPublic SQL query count = %d, want 1", got)
	}
}

func categoryPtr(category entity.CategoryCode) *entity.CategoryCode {
	return &category
}
