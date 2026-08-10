package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"coffee-reel/db"
	"coffee-reel/entity"
	"coffee-reel/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	seedPassword       = "SeedCoffee123!"
	seedUserCount      = 5
	seedDurationMillis = 9500
)

type seedVideoState string

const (
	seedVideoPublic  seedVideoState = "public"
	seedVideoPrivate seedVideoState = "private"
	seedVideoHidden  seedVideoState = "hidden"
	seedVideoDeleted seedVideoState = "deleted"
)

type seedVideoDefinition struct {
	Title       string
	Description string
	Category    entity.CategoryCode
	State       seedVideoState
}

type seedObjectKeys struct {
	Original  string
	Video     string
	Thumbnail string
}

type seedAsset struct {
	VideoPath     string
	ThumbnailPath string
	SizeBytes     int64
}

//go:embed assets/*.mp4
var seedAssets embed.FS

var seedCategories = []entity.CategoryCode{
	entity.CategoryBrewing,
	entity.CategoryRoasting,
	entity.CategoryLatteArt,
	entity.CategoryBeans,
	entity.CategoryEquipment,
}

func main() {
	production := isProductionEnvironment()

	if production && !productionSeedAllowed() {
		log.Fatal(
			"seed cannot run in production without ALLOW_PRODUCTION_SEED=true",
		)
	}

	if production {
		log.Println("production seed explicitly enabled")
	}

	ctx := context.Background()

	postgresDB, err := db.NewDB(requiredEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println(err)
		}
	}()

	var videoCount int64
	if err := postgresDB.Model(&entity.Video{}).Count(&videoCount).Error; err != nil {
		log.Fatal(fmt.Errorf("count existing videos: %w", err))
	}

	if videoCount > 0 {
		log.Printf("browser seed skipped: %d videos already exist", videoCount)
		return
	}

	storageRepository, err := repository.NewObjectStorageRepository(
		ctx,
		objectStorageConfig(),
	)
	if err != nil {
		log.Fatal(err)
	}

	mediaRepository, err := repository.NewMediaRepository(
		requiredEnv("FFPROBE_PATH"),
		requiredEnv("FFMPEG_PATH"),
	)
	if err != nil {
		log.Fatal(err)
	}

	users, err := ensureSeedUsers(ctx, postgresDB)
	if err != nil {
		log.Fatal(err)
	}

	assets, cleanupAssets, err := materializeSeedAssets(
		ctx,
		mediaRepository,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanupAssets()

	definitions := seedVideoDefinitions()

	storagePrefix := normalizeStoragePrefix(
		requiredEnv("STORAGE_MANAGED_PREFIX"),
	)
	if storagePrefix == "" {
		log.Fatal("STORAGE_MANAGED_PREFIX is invalid")
	}

	newObjectKeys, err := uploadSeedAssets(
		ctx,
		storageRepository,
		storagePrefix,
		definitions,
		assets,
	)
	if err != nil {
		log.Fatal(err)
	}

	publicVideos, err := seedDatabase(
		postgresDB,
		users,
		storagePrefix,
		definitions,
		assets,
	)
	if err != nil {
		cleanupNewObjects(
			ctx,
			storageRepository,
			newObjectKeys,
		)
		log.Fatal(err)
	}

	log.Printf(
		"browser seed completed: %d public videos + %d non-public videos",
		len(publicVideos),
		len(definitions)-len(publicVideos),
	)
}

func ensureSeedUsers(
	ctx context.Context,
	postgresDB *gorm.DB,
) ([]entity.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(seedPassword),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, fmt.Errorf("hash seed password: %w", err)
	}

	userRepository := repository.NewUserRepository(postgresDB)
	users := make([]entity.User, 0, seedUserCount)

	for index := 1; index <= seedUserCount; index++ {
		email := seedUserEmail(index)

		user, err := userRepository.FindByEmail(ctx, email)
		if err == nil {
			users = append(users, *user)
			continue
		}
		if !errors.Is(err, entity.ErrUserNotFound) {
			return nil, fmt.Errorf(
				"find seed user %d: %w",
				index,
				err,
			)
		}

		now := time.Now()

		user = &entity.User{
			Name:         fmt.Sprintf("Seed User %02d", index),
			Email:        email,
			PasswordHash: string(passwordHash),
			Role:         entity.RoleUser,
			Status:       entity.StatusActive,
			TokenVersion: 0,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := userRepository.Create(ctx, user); err != nil {
			return nil, fmt.Errorf(
				"create seed user %d: %w",
				index,
				err,
			)
		}

		users = append(users, *user)
	}

	return users, nil
}

func seedDatabase(
	postgresDB *gorm.DB,
	users []entity.User,
	storagePrefix string,
	definitions []seedVideoDefinition,
	assets map[entity.CategoryCode]seedAsset,
) ([]entity.Video, error) {
	if len(users) != seedUserCount {
		return nil, fmt.Errorf("seed users are incomplete")
	}

	publicVideos := make([]entity.Video, 0, 25)
	baseCreatedAt := time.Now().Add(-10 * time.Minute)

	err := postgresDB.Transaction(func(tx *gorm.DB) error {
		for index, definition := range definitions {
			asset, ok := assets[definition.Category]
			if !ok {
				return fmt.Errorf(
					"seed asset is missing for category: %s",
					definition.Category,
				)
			}

			owner := users[index%len(users)]
			createdAt := baseCreatedAt.Add(
				-time.Duration(index) * time.Minute,
			)
			keys := seedKeys(
				storagePrefix,
				definition.Category,
				index+1,
			)

			video, err := createSeedVideo(
				tx,
				owner.ID,
				definition,
				keys,
				asset,
				createdAt,
			)
			if err != nil {
				return fmt.Errorf(
					"create seed video %d: %w",
					index+1,
					err,
				)
			}

			if definition.State == seedVideoPublic {
				publicVideos = append(publicVideos, *video)
			}
		}

		for index, video := range publicVideos {
			likeCount := index % (seedUserCount + 1)

			for userIndex := 0; userIndex < likeCount; userIndex++ {
				like, err := entity.NewVideoLike(
					users[userIndex].ID,
					video.ID,
					baseCreatedAt.Add(
						time.Duration(index+userIndex)*
							time.Second,
					),
				)
				if err != nil {
					return err
				}

				if err := tx.Create(like).Error; err != nil {
					return fmt.Errorf(
						"create seed like: %w",
						err,
					)
				}
			}
		}

		for index := 0; index < 5 && index < len(publicVideos); index++ {
			saved, err := entity.NewSavedVideo(
				users[0].ID,
				publicVideos[index].ID,
				baseCreatedAt.Add(
					time.Duration(index)*time.Second,
				),
			)
			if err != nil {
				return err
			}

			if err := tx.Create(saved).Error; err != nil {
				return fmt.Errorf(
					"create seed saved video: %w",
					err,
				)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return publicVideos, nil
}

func createSeedVideo(
	tx *gorm.DB,
	ownerID uint64,
	definition seedVideoDefinition,
	keys seedObjectKeys,
	asset seedAsset,
	createdAt time.Time,
) (*entity.Video, error) {
	video, err := entity.NewVideo(
		ownerID,
		definition.Category,
		definition.Title,
		definition.Description,
		keys.Original,
		createdAt.Add(2*time.Hour),
		createdAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Create(video).Error; err != nil {
		return nil, fmt.Errorf("insert video: %w", err)
	}

	uploadedAt := createdAt.Add(time.Second)
	processingAt := createdAt.Add(2 * time.Second)
	sourceAt := createdAt.Add(3 * time.Second)
	completedAt := createdAt.Add(4 * time.Second)

	if err := video.CompleteUpload(uploadedAt); err != nil {
		return nil, err
	}

	if err := video.StartProcessing(processingAt); err != nil {
		return nil, err
	}

	sourceMeta := entity.SourceVideoMeta{
		MIMEType:       "video/mp4",
		Container:      "mp4",
		SizeBytes:      asset.SizeBytes,
		DurationMillis: seedDurationMillis,
		Width:          720,
		Height:         1280,
		FrameRate:      24,
		VideoCodec:     "h264",
		HasAudio:       true,
		AudioCodec:     "aac",
	}

	if err := video.RecordSourceValidation(
		sourceMeta,
		sourceAt,
	); err != nil {
		return nil, err
	}

	outputMeta := entity.OutputVideoMeta{
		VideoObjectKey:     keys.Video,
		ThumbnailObjectKey: keys.Thumbnail,
		Container:          "mp4",
		Width:              720,
		Height:             1280,
		FrameRate:          24,
		VideoCodec:         "h264",
		HasAudio:           true,
		AudioCodec:         "aac",
	}

	if err := video.CompleteProcessing(
		outputMeta,
		true,
		completedAt,
	); err != nil {
		return nil, err
	}

	switch definition.State {
	case seedVideoPublic:
	case seedVideoPrivate:
		if err := video.SetPrivateByOwner(
			ownerID,
			completedAt.Add(time.Second),
		); err != nil {
			return nil, err
		}
	case seedVideoHidden:
		if err := video.HideByAdmin(
			completedAt.Add(time.Second),
		); err != nil {
			return nil, err
		}
	case seedVideoDeleted:
		if err := video.DeleteByOwner(
			ownerID,
			completedAt.Add(time.Second),
		); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf(
			"unsupported seed video state: %s",
			definition.State,
		)
	}

	if err := tx.Create(video.SourceMeta).Error; err != nil {
		return nil, fmt.Errorf(
			"insert source meta: %w",
			err,
		)
	}

	if err := tx.Create(video.OutputMeta).Error; err != nil {
		return nil, fmt.Errorf(
			"insert output meta: %w",
			err,
		)
	}

	if err := tx.Model(&entity.Video{}).
		Where("id = ?", video.ID).
		Select(
			"processing_status",
			"publish_status",
			"updated_at",
			"deleted_at",
		).
		Updates(video).Error; err != nil {
		return nil, fmt.Errorf(
			"save final seed video state: %w",
			err,
		)
	}

	return video, nil
}

func uploadSeedAssets(
	ctx context.Context,
	storage repository.IObjectStorageRepository,
	storagePrefix string,
	definitions []seedVideoDefinition,
	assets map[entity.CategoryCode]seedAsset,
) ([]string, error) {
	newObjectKeys := make(
		[]string,
		0,
		len(definitions)*3,
	)

	for index, definition := range definitions {
		asset, ok := assets[definition.Category]
		if !ok {
			cleanupNewObjects(
				ctx,
				storage,
				newObjectKeys,
			)
			return nil, fmt.Errorf(
				"seed asset is missing for category: %s",
				definition.Category,
			)
		}

		keys := seedKeys(
			storagePrefix,
			definition.Category,
			index+1,
		)

		objects := []struct {
			key       string
			path      string
			thumbnail bool
		}{
			{
				key:  keys.Original,
				path: asset.VideoPath,
			},
			{
				key:  keys.Video,
				path: asset.VideoPath,
			},
			{
				key:       keys.Thumbnail,
				path:      asset.ThumbnailPath,
				thumbnail: true,
			},
		}

		for _, object := range objects {
			exists, err := storage.Exists(
				ctx,
				object.key,
			)
			if err != nil {
				cleanupNewObjects(
					ctx,
					storage,
					newObjectKeys,
				)
				return nil, fmt.Errorf(
					"check seed object %s: %w",
					object.key,
					err,
				)
			}

			if object.thumbnail {
				err = storage.UploadThumbnail(
					ctx,
					object.key,
					object.path,
				)
			} else {
				err = storage.UploadProcessed(
					ctx,
					object.key,
					object.path,
				)
			}
			if err != nil {
				cleanupNewObjects(
					ctx,
					storage,
					newObjectKeys,
				)
				return nil, fmt.Errorf(
					"upload seed object %s: %w",
					object.key,
					err,
				)
			}

			if !exists {
				newObjectKeys = append(
					newObjectKeys,
					object.key,
				)
			}
		}
	}

	return newObjectKeys, nil
}

func cleanupNewObjects(
	ctx context.Context,
	storage repository.IObjectStorageRepository,
	objectKeys []string,
) {
	for index := len(objectKeys) - 1; index >= 0; index-- {
		if err := storage.Delete(
			ctx,
			objectKeys[index],
		); err != nil {
			log.Printf(
				"cleanup seed object failed: %s",
				objectKeys[index],
			)
		}
	}
}

func materializeSeedAssets(
	ctx context.Context,
	media repository.IMediaRepository,
) (
	map[entity.CategoryCode]seedAsset,
	func(),
	error,
) {
	tempDir, err := os.MkdirTemp(
		"",
		"coffee-reel-browser-seed-",
	)
	if err != nil {
		return nil, func() {}, fmt.Errorf(
			"create seed temp directory: %w",
			err,
		)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	assets := make(
		map[entity.CategoryCode]seedAsset,
		len(seedCategories),
	)

	for _, category := range seedCategories {
		baseName := string(category)

		videoBytes, err := seedAssets.ReadFile(
			"assets/" + baseName + ".mp4",
		)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf(
				"read %s seed video: %w",
				category,
				err,
			)
		}

		videoPath := filepath.Join(
			tempDir,
			baseName+".mp4",
		)
		thumbnailPath := filepath.Join(
			tempDir,
			baseName+".jpg",
		)

		if err := os.WriteFile(
			videoPath,
			videoBytes,
			0o600,
		); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf(
				"write %s seed video: %w",
				category,
				err,
			)
		}

		if err := media.GenerateThumbnail(
			ctx,
			videoPath,
			thumbnailPath,
		); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf(
				"generate %s seed thumbnail: %w",
				category,
				err,
			)
		}

		assets[category] = seedAsset{
			VideoPath:     videoPath,
			ThumbnailPath: thumbnailPath,
			SizeBytes:     int64(len(videoBytes)),
		}
	}

	return assets, cleanup, nil
}

func seedKeys(
	storagePrefix string,
	category entity.CategoryCode,
	index int,
) seedObjectKeys {
	base := fmt.Sprintf(
		"%s/browser-seed/%02d-%s",
		storagePrefix,
		index,
		category,
	)

	return seedObjectKeys{
		Original:  base + "/original.mp4",
		Video:     base + "/output.mp4",
		Thumbnail: base + "/thumbnail.jpg",
	}
}

func seedUserEmail(index int) string {
	return fmt.Sprintf(
		"seed%02d@example.com",
		index,
	)
}

func normalizeStoragePrefix(value string) string {
	value = strings.Trim(
		strings.TrimSpace(value),
		"/",
	)

	if value == "" ||
		strings.Contains(value, "\\") {
		return ""
	}

	return value
}

func seedVideoDefinitions() []seedVideoDefinition {
	return []seedVideoDefinition{
		{
			Title:       "V60 ハンドドリップの基本",
			Description: "湯温と注ぎ方を整えて、毎日のドリップを安定させる基本手順。",
			Category:    entity.CategoryBrewing,
			State:       seedVideoPublic,
		},
		{
			Title:       "ハンドドリップ 初心者向けレシピ",
			Description: "粉量、湯量、抽出時間をシンプルに決める初心者向けレシピ。",
			Category:    entity.CategoryBrewing,
			State:       seedVideoPublic,
		},
		{
			Title:       "急冷アイスコーヒーの淹れ方",
			Description: "氷へ直接落として香りを残す急冷ドリップのポイント。",
			Category:    entity.CategoryBrewing,
			State:       seedVideoPublic,
		},
		{
			Title:       "フレンチプレスで甘さを引き出す",
			Description: "浸漬式で雑味を抑えながら甘さを出す基本の考え方。",
			Category:    entity.CategoryBrewing,
			State:       seedVideoPublic,
		},
		{
			Title:       "エアロプレス 3分レシピ",
			Description: "短時間で再現しやすいエアロプレスの抽出レシピ。",
			Category:    entity.CategoryBrewing,
			State:       seedVideoPublic,
		},

		{
			Title:       "浅煎り焙煎の考え方",
			Description: "酸の明るさを残すための浅煎り焙煎の基本。",
			Category:    entity.CategoryRoasting,
			State:       seedVideoPublic,
		},
		{
			Title:       "中煎りで甘さを作る",
			Description: "香りと甘さのバランスを狙う中煎りの進め方。",
			Category:    entity.CategoryRoasting,
			State:       seedVideoPublic,
		},
		{
			Title:       "深煎りの火力調整",
			Description: "焦げを避けながらコクを作る火力調整のポイント。",
			Category:    entity.CategoryRoasting,
			State:       seedVideoPublic,
		},
		{
			Title:       "1ハゼを見極める",
			Description: "焙煎中の音と香りから1ハゼを判断する方法。",
			Category:    entity.CategoryRoasting,
			State:       seedVideoPublic,
		},
		{
			Title:       "焙煎後のエイジング",
			Description: "焙煎直後から飲み頃までの変化を確認する。",
			Category:    entity.CategoryRoasting,
			State:       seedVideoPublic,
		},

		{
			Title:       "ラテアート ハートの基本",
			Description: "ミルクの流量とカップ角度を合わせてハートを描く。",
			Category:    entity.CategoryLatteArt,
			State:       seedVideoPublic,
		},
		{
			Title:       "リーフをきれいに描く",
			Description: "左右の振り幅を安定させてリーフを整える練習。",
			Category:    entity.CategoryLatteArt,
			State:       seedVideoPublic,
		},
		{
			Title:       "チューリップの重ね方",
			Description: "複数のハートを重ねてチューリップを作る手順。",
			Category:    entity.CategoryLatteArt,
			State:       seedVideoPublic,
		},
		{
			Title:       "ミルクスチームの基本",
			Description: "きめ細かなフォームを作るための空気の入れ方。",
			Category:    entity.CategoryLatteArt,
			State:       seedVideoPublic,
		},
		{
			Title:       "ラテアート 練習ルーティン",
			Description: "短時間でも毎日続けやすいラテアート練習方法。",
			Category:    entity.CategoryLatteArt,
			State:       seedVideoPublic,
		},

		{
			Title:       "エチオピア ナチュラルの香り",
			Description: "ベリー系の香りが出やすい豆の特徴を確認する。",
			Category:    entity.CategoryBeans,
			State:       seedVideoPublic,
		},
		{
			Title:       "コロンビア豆の選び方",
			Description: "酸味と甘さのバランスからコロンビア豆を選ぶ。",
			Category:    entity.CategoryBeans,
			State:       seedVideoPublic,
		},
		{
			Title:       "ケニアの明るい酸味",
			Description: "ケニア豆に感じやすい果実感と酸の特徴。",
			Category:    entity.CategoryBeans,
			State:       seedVideoPublic,
		},
		{
			Title:       "ブラジル豆のナッツ感",
			Description: "日常使いしやすいブラジル豆の風味を紹介する。",
			Category:    entity.CategoryBeans,
			State:       seedVideoPublic,
		},
		{
			Title:       "ゲイシャのフローラル香",
			Description: "華やかな香りを持つゲイシャの特徴を確認する。",
			Category:    entity.CategoryBeans,
			State:       seedVideoPublic,
		},

		{
			Title:       "コーヒーグラインダーの選び方",
			Description: "粒度調整と使い方からグラインダーを比較する。",
			Category:    entity.CategoryEquipment,
			State:       seedVideoPublic,
		},
		{
			Title:       "V60 ドリッパー比較",
			Description: "素材やサイズによるV60ドリッパーの違いを見る。",
			Category:    entity.CategoryEquipment,
			State:       seedVideoPublic,
		},
		{
			Title:       "細口ケトルの使い方",
			Description: "湯量を安定させるための細口ケトルの持ち方。",
			Category:    entity.CategoryEquipment,
			State:       seedVideoPublic,
		},
		{
			Title:       "コーヒースケール入門",
			Description: "抽出量と時間を同時に管理するスケールの基本。",
			Category:    entity.CategoryEquipment,
			State:       seedVideoPublic,
		},
		{
			Title:       "エスプレッソマシンの日常清掃",
			Description: "抽出品質を保つための日常的な清掃ポイント。",
			Category:    entity.CategoryEquipment,
			State:       seedVideoPublic,
		},

		{
			Title:       "非公開テスト V60 レシピ",
			Description: "検索結果へ出てはいけないprivate動画。",
			Category:    entity.CategoryBrewing,
			State:       seedVideoPrivate,
		},
		{
			Title:       "管理者非表示テスト ラテアート",
			Description: "検索結果へ出てはいけないhidden動画。",
			Category:    entity.CategoryLatteArt,
			State:       seedVideoHidden,
		},
		{
			Title:       "削除済みテスト グラインダー",
			Description: "検索結果へ出てはいけないdeleted動画。",
			Category:    entity.CategoryEquipment,
			State:       seedVideoDeleted,
		},
	}
}

func objectStorageConfig() repository.ObjectStorageConfig {
	provider := strings.ToLower(
		requiredEnv("STORAGE_PROVIDER"),
	)
	if provider != "s3" {
		log.Fatal("STORAGE_PROVIDER must be s3")
	}

	return repository.ObjectStorageConfig{
		Endpoint: requiredEnv(
			"STORAGE_ENDPOINT",
		),
		PresignEndpoint: strings.TrimSpace(
			os.Getenv("STORAGE_PRESIGN_ENDPOINT"),
		),
		Region: requiredEnv(
			"STORAGE_REGION",
		),
		Bucket: requiredEnv(
			"STORAGE_BUCKET",
		),
		AccessKeyID: requiredEnv(
			"STORAGE_ACCESS_KEY_ID",
		),
		SecretAccessKey: requiredEnv(
			"STORAGE_SECRET_ACCESS_KEY",
		),
		ManagedPrefix: requiredEnv(
			"STORAGE_MANAGED_PREFIX",
		),
		ForcePathStyle: requiredBoolEnv(
			"STORAGE_FORCE_PATH_STYLE",
		),
		RequireHTTPS: false,
	}
}

func isProductionEnvironment() bool {
	environment := strings.ToLower(
		strings.TrimSpace(os.Getenv("GO_ENV")),
	)

	return environment == "production" ||
		environment == "prod"
}

func productionSeedAllowed() bool {
	return strings.EqualFold(
		strings.TrimSpace(
			os.Getenv("ALLOW_PRODUCTION_SEED"),
		),
		"true",
	)
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(
		os.Getenv(name),
	)
	if value == "" {
		log.Fatal(name + " is required")
	}

	return value
}

func requiredBoolEnv(name string) bool {
	value := requiredEnv(name)

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatal(name + " must be true or false")
	}

	return parsed
}
