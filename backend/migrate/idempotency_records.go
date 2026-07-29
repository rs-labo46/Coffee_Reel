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
	if err := addForeignKey(db, "idempotency_records", "fk_idempotency_records_user", "user_id", "users", "id", "CASCADE"); err != nil {
		return err
	}
	if err := addForeignKey(db, "idempotency_records", "fk_idempotency_records_video", "resource_id", "videos", "id", "CASCADE"); err != nil {
		return err
	}
	if err := addCheck(db, "idempotency_records", "chk_idempotency_records_scope", "scope = 'video_create'"); err != nil {
		return err
	}
	if err := addCheck(db, "idempotency_records", "chk_idempotency_records_hashes", "char_length(key_hash) = 64 AND char_length(request_hash) = 64"); err != nil {
		return err
	}
	if err := addCheck(db, "idempotency_records", "chk_idempotency_records_expiry", "expires_at > created_at"); err != nil {
		return err
	}
	return nil
}
