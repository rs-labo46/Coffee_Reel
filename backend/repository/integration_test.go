//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"coffee-reel/migrate"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var integrationSchemaCounter atomic.Uint64

func openPostgresIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL or DATABASE_URL is required for PostgreSQL integration tests")
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	adminSQL := stdlib.OpenDB(*cfg)
	if err := adminSQL.PingContext(context.Background()); err != nil {
		_ = adminSQL.Close()
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	schema := fmt.Sprintf("coffee_reel_test_%d_%d", time.Now().UnixNano(), integrationSchemaCounter.Add(1))
	if _, err := adminSQL.ExecContext(context.Background(), `CREATE SCHEMA "`+schema+`"`); err != nil {
		_ = adminSQL.Close()
		t.Fatalf("create test schema: %v", err)
	}

	testCfg := cfg.Copy()
	if testCfg.RuntimeParams == nil {
		testCfg.RuntimeParams = make(map[string]string)
	}
	testCfg.RuntimeParams["search_path"] = schema + ",public"
	testSQL := stdlib.OpenDB(*testCfg)
	testSQL.SetMaxOpenConns(20)
	testSQL.SetMaxIdleConns(20)

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: testSQL}), &gorm.Config{
		NowFunc: func() time.Time { return time.Now() },
	})
	if err != nil {
		_ = testSQL.Close()
		_, _ = adminSQL.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
		_ = adminSQL.Close()
		t.Fatalf("open GORM test database: %v", err)
	}

	if err := migrate.MigrateUsers(db); err != nil {
		t.Fatalf("MigrateUsers: %v", err)
	}
	if err := migrate.MigrateRefreshTokens(db); err != nil {
		t.Fatalf("MigrateRefreshTokens: %v", err)
	}

	if err := migrate.MigrateAdminAuditLogs(db); err != nil {
		t.Fatalf("MigrateAdminAuditLogs: %v", err)
	}
	if err := migrate.MigrateVideos(db); err != nil {
		t.Fatalf("MigrateVideos: %v", err)
	}

	t.Cleanup(func() {
		_ = testSQL.Close()
		_, _ = adminSQL.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
		_ = adminSQL.Close()
	})

	return db
}

func openRedisIntegrationClient(t *testing.T) (*redis.Client, string) {
	t.Helper()

	addr := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR"))
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR is required for Redis integration tests")
	}
	dbNumber := 15
	if raw := strings.TrimSpace(os.Getenv("TEST_REDIS_DB")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("TEST_REDIS_DB must be an integer: %v", err)
		}
		dbNumber = parsed
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
		DB:       dbNumber,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("connect to Redis: %v", err)
	}

	prefix := fmt.Sprintf("coffee-reel-test:%d:%d:", time.Now().UnixNano(), integrationSchemaCounter.Add(1))
	t.Cleanup(func() {
		deleteRedisKeysByPrefix(client, prefix)
		_ = client.Close()
	})
	return client, prefix
}

func deleteRedisKeysByPrefix(client *redis.Client, prefix string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func closeSQLDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	if err := sqlDB.Close(); err != nil && err != sql.ErrConnDone {
		t.Fatalf("close SQL DB: %v", err)
	}
}
