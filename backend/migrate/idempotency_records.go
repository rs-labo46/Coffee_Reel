package migrate

import (
	"coffee-reel/entity"
	"fmt"

	"gorm.io/gorm"
)

func MigrateIdempotencyRecords(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}

	if !db.Migrator().HasTable(&entity.IdempotencyRecord{}) {
		if err := db.Migrator().CreateTable(&entity.IdempotencyRecord{}); err != nil {
			return fmt.Errorf("create idempotency records table: %w", err)
		}
	}

	if err := addForeignKey(
		db,
		"idempotency_records",
		"fk_idempotency_records_user",
		"user_id",
		"users",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}
	if err := addForeignKey(
		db,
		"idempotency_records",
		"fk_idempotency_records_video",
		"resource_id",
		"videos",
		"id",
		"CASCADE",
	); err != nil {
		return err
	}

	checks := []migrationCheck{
		{
			name:       "chk_idempotency_scope",
			expression: "scope = 'video_create'",
		},
		{
			name:       "chk_idempotency_key_hash",
			expression: "key_hash ~ '^[0-9a-f]{64}$'",
		},
		{
			name:       "chk_idempotency_request_hash",
			expression: "request_hash ~ '^[0-9a-f]{64}$'",
		},
		{
			name:       "chk_idempotency_expiry",
			expression: "expires_at > created_at",
		},
	}
	if err := addChecks(db, "idempotency_records", checks); err != nil {
		return err
	}

	indexes := []migrationIndex{
		{
			name:      "uq_idempotency_records_user_scope_key",
			statement: "CREATE UNIQUE INDEX uq_idempotency_records_user_scope_key ON idempotency_records (user_id, scope, key_hash)",
		},
		{
			name:      "idx_idempotency_records_expires_at",
			statement: "CREATE INDEX idx_idempotency_records_expires_at ON idempotency_records (expires_at)",
		},
	}
	return createIndexesIfMissing(db, indexes)
}
