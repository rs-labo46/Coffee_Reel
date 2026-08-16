package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"coffee-reel/db"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	benchmarkUserCount    = 5
	benchmarkObjectPrefix = "videos/benchmark-search/"
	benchmarkEmailPattern = "benchmark-search-%@local.invalid"
)

var supportedCounts = []int{
	100,
	1000,
	10000,
}

type benchmarkCounts struct {
	Videos      int64 `gorm:"column:videos"`
	OutputMetas int64 `gorm:"column:output_metas"`
	Likes       int64 `gorm:"column:likes"`
}

func main() {
	count := flag.Int(
		"count",
		1000,
		"benchmark video count: 100, 1000, or 10000",
	)
	cleanup := flag.Bool(
		"cleanup",
		false,
		"remove all search benchmark data",
	)
	confirmLocal := flag.Bool(
		"confirm-local",
		false,
		"required safety flag for local benchmark execution",
	)
	flag.Parse()

	if !*confirmLocal {
		log.Fatal(
			"search benchmark seed requires --confirm-local",
		)
	}

	if isProductionEnvironment() {
		log.Fatal(
			"search benchmark seed cannot run in production",
		)
	}

	if !*cleanup && !isSupportedCount(*count) {
		log.Fatalf(
			"unsupported benchmark count: %d; allowed values are 100, 1000, 10000",
			*count,
		)
	}

	postgresDB, err := db.NewDB(
		requiredEnv("DATABASE_URL"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println(err)
		}
	}()

	if err := verifySearchPrerequisites(postgresDB); err != nil {
		log.Fatal(err)
	}

	if *cleanup {
		if err := cleanupAllBenchmarkData(postgresDB); err != nil {
			log.Fatal(err)
		}

		if err := vacuumBenchmarkTables(postgresDB); err != nil {
			log.Fatal(err)
		}

		log.Println(
			"search benchmark data cleanup completed",
		)
		return
	}

	if err := seedBenchmarkData(postgresDB, *count); err != nil {
		log.Fatal(err)
	}

	if err := vacuumBenchmarkTables(postgresDB); err != nil {
		log.Fatal(err)
	}

	counts, err := readBenchmarkCounts(postgresDB)
	if err != nil {
		log.Fatal(err)
	}

	expectedLikes := expectedLikeCount(*count)

	if counts.Videos != int64(*count) {
		log.Fatalf(
			"benchmark video count mismatch: expected=%d actual=%d",
			*count,
			counts.Videos,
		)
	}

	if counts.OutputMetas != int64(*count) {
		log.Fatalf(
			"benchmark output meta count mismatch: expected=%d actual=%d",
			*count,
			counts.OutputMetas,
		)
	}

	if counts.Likes != expectedLikes {
		log.Fatalf(
			"benchmark like count mismatch: expected=%d actual=%d",
			expectedLikes,
			counts.Likes,
		)
	}

	log.Printf(
		"search benchmark seed completed: videos=%d output_metas=%d likes=%d",
		counts.Videos,
		counts.OutputMetas,
		counts.Likes,
	)

	log.Println(
		"benchmark title match query: latte-art",
	)
	log.Println(
		"benchmark similarity fallback query: latte-artt",
	)
}

func seedBenchmarkData(
	postgresDB *gorm.DB,
	count int,
) error {
	passwordHash, err := createRandomPasswordHash()
	if err != nil {
		return err
	}

	return postgresDB.Transaction(
		func(tx *gorm.DB) error {
			if err := deleteBenchmarkVideos(tx); err != nil {
				return err
			}

			userIDs, err := ensureBenchmarkUsers(
				tx,
				passwordHash,
			)
			if err != nil {
				return err
			}

			if len(userIDs) != benchmarkUserCount {
				return fmt.Errorf(
					"benchmark users are incomplete",
				)
			}

			if err := insertBenchmarkVideos(
				tx,
				userIDs[0],
				count,
			); err != nil {
				return err
			}

			if err := insertBenchmarkLikes(
				tx,
				count,
			); err != nil {
				return err
			}

			return nil
		},
	)
}

func ensureBenchmarkUsers(
	tx *gorm.DB,
	passwordHash string,
) ([]uint64, error) {
	now := time.Now()
	userIDs := make(
		[]uint64,
		0,
		benchmarkUserCount,
	)

	for index := 1; index <= benchmarkUserCount; index++ {
		email := fmt.Sprintf(
			"benchmark-search-%02d@local.invalid",
			index,
		)
		name := fmt.Sprintf(
			"Benchmark User %02d",
			index,
		)

		var userID uint64

		err := tx.Raw(
			`
INSERT INTO users (
	name,
	email,
	password_hash,
	role,
	status,
	token_version,
	created_at,
	updated_at
)
VALUES (?, ?, ?, 'user', 'active', 0, ?, ?)
ON CONFLICT (email)
DO UPDATE SET
	name = EXCLUDED.name,
	role = 'user',
	status = 'active',
	updated_at = EXCLUDED.updated_at
RETURNING id
`,
			name,
			email,
			passwordHash,
			now,
			now,
		).
			Scan(&userID).
			Error
		if err != nil {
			return nil, fmt.Errorf(
				"ensure benchmark user %d: %w",
				index,
				err,
			)
		}

		if userID == 0 {
			return nil, fmt.Errorf(
				"benchmark user %d has invalid id",
				index,
			)
		}

		userIDs = append(
			userIDs,
			userID,
		)
	}

	return userIDs, nil
}

func insertBenchmarkVideos(
	tx *gorm.DB,
	ownerUserID uint64,
	count int,
) error {
	if ownerUserID == 0 {
		return fmt.Errorf(
			"benchmark owner user id is required",
		)
	}

	if !isSupportedCount(count) {
		return fmt.Errorf(
			"unsupported benchmark count: %d",
			count,
		)
	}

	err := tx.Exec(
		`
WITH generated AS (
	SELECT
		n,
		CASE ((n - 1) % 5)
			WHEN 0 THEN 'brewing'
			WHEN 1 THEN 'roasting'
			WHEN 2 THEN 'latte_art'
			WHEN 3 THEN 'beans'
			ELSE 'equipment'
		END AS category,
		CASE
			WHEN ((n - 1) % 5) = 0 THEN
				'Benchmark ハンドドリップ brewing ' ||
				lpad(n::text, 6, '0')
			WHEN ((n - 1) % 5) = 1 THEN
				'Benchmark コーヒー焙煎 roasting ' ||
				lpad(n::text, 6, '0')
			WHEN ((n - 1) % 5) = 2
				AND ((n - 3) % 50) = 0 THEN
				'Benchmark ラテアート latte-art rosetta ' ||
				lpad(n::text, 6, '0')
			WHEN ((n - 1) % 5) = 2 THEN
				'Benchmark ラテアート milk-foam ' ||
				lpad(n::text, 6, '0')
			WHEN ((n - 1) % 5) = 3 THEN
				'Benchmark コーヒー豆 beans ' ||
				lpad(n::text, 6, '0')
			ELSE
				'Benchmark コーヒー器具 equipment ' ||
				lpad(n::text, 6, '0')
		END AS title,
		CURRENT_TIMESTAMP -
			(n * INTERVAL '1 second') AS created_at
	FROM generate_series(
		1,
		CAST(? AS integer)
	) AS series(n)
),
inserted AS (
	INSERT INTO videos (
		user_id,
		category,
		title,
		description,
		original_object_key,
		upload_expires_at,
		processing_status,
		publish_status,
		created_at,
		updated_at,
		deleted_at
	)
	SELECT
		?,
		generated.category,
		generated.title,
		'Search scalability benchmark row ' ||
			generated.n::text,
		'videos/benchmark-search/source/' ||
			lpad(generated.n::text, 6, '0') ||
			'.mp4',
		generated.created_at +
			INTERVAL '15 minutes',
		'ready',
		'published',
		generated.created_at,
		generated.created_at,
		NULL
	FROM generated
	RETURNING id, created_at
)
INSERT INTO video_output_metas (
	video_id,
	video_object_key,
	thumbnail_object_key,
	container,
	width,
	height,
	frame_rate,
	video_codec,
	has_audio,
	audio_codec,
	created_at
)
SELECT
	inserted.id,
	'videos/benchmark-search/' ||
		inserted.id::text ||
		'/output.mp4',
	'videos/benchmark-search/' ||
		inserted.id::text ||
		'/thumbnail.jpg',
	'mp4',
	720,
	1280,
	30,
	'h264',
	FALSE,
	'',
	inserted.created_at
FROM inserted
`,
		count,
		ownerUserID,
	).Error
	if err != nil {
		return fmt.Errorf(
			"insert benchmark videos: %w",
			err,
		)
	}

	return nil
}

func insertBenchmarkLikes(
	tx *gorm.DB,
	count int,
) error {
	if !isSupportedCount(count) {
		return fmt.Errorf(
			"unsupported benchmark count: %d",
			count,
		)
	}

	err := tx.Exec(
		`
WITH ranked_users AS (
	SELECT
		id,
		row_number() OVER (
			ORDER BY email ASC
		) AS rn
	FROM users
	WHERE email LIKE 'benchmark-search-%@local.invalid'
),
ranked_videos AS (
	SELECT
		id,
		created_at,
		row_number() OVER (
			ORDER BY id ASC
		) AS rn
	FROM videos
	WHERE original_object_key LIKE
		'videos/benchmark-search/%'
)
INSERT INTO video_likes (
	user_id,
	video_id,
	created_at
)
SELECT
	ranked_users.id,
	ranked_videos.id,
	ranked_videos.created_at
FROM ranked_videos
JOIN ranked_users
	ON ranked_users.rn <= MOD(
		ranked_videos.rn - 1,
		CAST(? AS bigint)
	)
`,
		benchmarkUserCount,
	).Error
	if err != nil {
		return fmt.Errorf(
			"insert benchmark likes: %w",
			err,
		)
	}

	return nil
}

func deleteBenchmarkVideos(
	tx *gorm.DB,
) error {
	if err := tx.Exec(
		`
DELETE FROM videos
WHERE original_object_key LIKE
	'videos/benchmark-search/%'
`,
	).Error; err != nil {
		return fmt.Errorf(
			"delete benchmark videos: %w",
			err,
		)
	}

	return nil
}

func cleanupAllBenchmarkData(
	postgresDB *gorm.DB,
) error {
	return postgresDB.Transaction(
		func(tx *gorm.DB) error {
			if err := deleteBenchmarkVideos(tx); err != nil {
				return err
			}

			if err := tx.Exec(
				`
DELETE FROM users
WHERE email LIKE
	'benchmark-search-%@local.invalid'
`,
			).Error; err != nil {
				return fmt.Errorf(
					"delete benchmark users: %w",
					err,
				)
			}

			return nil
		},
	)
}

func verifySearchPrerequisites(
	postgresDB *gorm.DB,
) error {
	var pgTrgmExists bool

	if err := postgresDB.Raw(
		`
SELECT EXISTS (
	SELECT 1
	FROM pg_extension
	WHERE extname = 'pg_trgm'
)
`,
	).
		Scan(&pgTrgmExists).
		Error; err != nil {
		return fmt.Errorf(
			"check pg_trgm extension: %w",
			err,
		)
	}

	if !pgTrgmExists {
		return fmt.Errorf(
			"pg_trgm extension is not installed; run migrations first",
		)
	}

	indexNames := []string{
		"idx_videos_public_feed",
		"idx_videos_public_category",
		"idx_videos_public_title_trgm",
	}

	for _, indexName := range indexNames {
		exists, err := searchIndexExists(
			postgresDB,
			indexName,
		)
		if err != nil {
			return err
		}

		if !exists {
			return fmt.Errorf(
				"required search index is missing: %s",
				indexName,
			)
		}
	}

	return nil
}

func searchIndexExists(
	postgresDB *gorm.DB,
	indexName string,
) (bool, error) {
	var exists bool

	if err := postgresDB.Raw(
		`
SELECT EXISTS (
	SELECT 1
	FROM pg_indexes
	WHERE indexname = ?
)
`,
		indexName,
	).
		Scan(&exists).
		Error; err != nil {
		return false, fmt.Errorf(
			"check search index %s: %w",
			indexName,
			err,
		)
	}

	return exists, nil
}

func readBenchmarkCounts(
	postgresDB *gorm.DB,
) (benchmarkCounts, error) {
	var counts benchmarkCounts

	err := postgresDB.Raw(
		`
SELECT
	(
		SELECT COUNT(*)
		FROM videos
		WHERE original_object_key LIKE
			'videos/benchmark-search/%'
	) AS videos,
	(
		SELECT COUNT(*)
		FROM video_output_metas AS output
		JOIN videos AS video
			ON video.id = output.video_id
		WHERE video.original_object_key LIKE
			'videos/benchmark-search/%'
	) AS output_metas,
	(
		SELECT COUNT(*)
		FROM video_likes AS video_like
		JOIN videos AS video
			ON video.id = video_like.video_id
		WHERE video.original_object_key LIKE
			'videos/benchmark-search/%'
	) AS likes
`,
	).
		Scan(&counts).
		Error
	if err != nil {
		return benchmarkCounts{}, fmt.Errorf(
			"read benchmark counts: %w",
			err,
		)
	}

	return counts, nil
}

func vacuumBenchmarkTables(
	postgresDB *gorm.DB,
) error {
	if err := postgresDB.Exec(
		"VACUUM (ANALYZE) videos",
	).Error; err != nil {
		return fmt.Errorf(
			"vacuum videos: %w",
			err,
		)
	}

	if err := postgresDB.Exec(
		"VACUUM (ANALYZE) video_output_metas",
	).Error; err != nil {
		return fmt.Errorf(
			"vacuum video_output_metas: %w",
			err,
		)
	}

	if err := postgresDB.Exec(
		"VACUUM (ANALYZE) video_likes",
	).Error; err != nil {
		return fmt.Errorf(
			"vacuum video_likes: %w",
			err,
		)
	}

	return nil
}

func createRandomPasswordHash() (
	string,
	error,
) {
	randomValue := make([]byte, 32)

	if _, err := rand.Read(randomValue); err != nil {
		return "", fmt.Errorf(
			"generate benchmark password material: %w",
			err,
		)
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		randomValue,
		bcrypt.DefaultCost,
	)
	if err != nil {
		return "", fmt.Errorf(
			"hash benchmark password material: %w",
			err,
		)
	}

	return string(passwordHash), nil
}

func expectedLikeCount(
	count int,
) int64 {
	fullCycles := count / benchmarkUserCount
	remainder := count % benchmarkUserCount

	perCycle := benchmarkUserCount *
		(benchmarkUserCount - 1) /
		2

	remainderLikes := remainder *
		(remainder - 1) /
		2

	return int64(
		fullCycles*perCycle +
			remainderLikes,
	)
}

func isSupportedCount(
	count int,
) bool {
	for _, supportedCount := range supportedCounts {
		if count == supportedCount {
			return true
		}
	}

	return false
}

func requiredEnv(
	key string,
) string {
	value := strings.TrimSpace(
		os.Getenv(key),
	)
	if value == "" {
		log.Fatalf(
			"%s is required",
			key,
		)
	}

	return value
}

func isProductionEnvironment() bool {
	value := strings.ToLower(requiredEnv("ENVIRONMENT"))
	switch value {
	case "develop":
		return false
	case "production":
		return true
	default:
		log.Fatal("ENVIRONMENT must be develop or production")
	}

	return false
}
