//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"coffee-reel/entity"
)

func createIntegrationUser(t *testing.T, dbRepository IUserRepository, email string) *entity.User {
	t.Helper()
	user := newIntegrationUser(email)
	if err := dbRepository.Create(context.Background(), user); err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	return user
}

func newIntegrationRefreshToken(userID uint64, hash, family string, expiresAt time.Time) *entity.RefreshToken {
	return &entity.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		FamilyID:  family,
		ExpiresAt: expiresAt.Truncate(time.Microsecond),
		CreatedAt: time.Now().Truncate(time.Microsecond),
	}
}

func TestRefreshTokenRepositoryIntegrationCreateAndFind(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	user := createIntegrationUser(t, users, "refresh-create@example.com")
	token := newIntegrationRefreshToken(user.ID, "hash-create", "family-create", time.Now().Add(time.Hour))

	if err := repository.Create(context.Background(), token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if token.ID == 0 {
		t.Fatal("Create() did not assign token ID")
	}
	found, err := repository.FindByTokenHash(context.Background(), token.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash() error = %v", err)
	}
	if found.ID != token.ID || found.UserID != user.ID || found.FamilyID != token.FamilyID {
		t.Fatalf("found token = %+v", found)
	}
	if _, err := repository.FindByTokenHash(context.Background(), "missing"); !errors.Is(err, entity.ErrRefreshTokenInvalid) {
		t.Fatalf("FindByTokenHash(missing) error = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestRefreshTokenMigrationEnforcesUserAndReplacementForeignKeys(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewRefreshTokenRepository(db)

	orphanUserToken := newIntegrationRefreshToken(999999, "orphan-user", "family", time.Now().Add(time.Hour))
	if err := repository.Create(context.Background(), orphanUserToken); err == nil {
		t.Fatal("refresh_tokens.user_id accepted an unknown user; required foreign key is missing")
	}

	users := NewUserRepository(db)
	user := createIntegrationUser(t, users, "replacement-fk@example.com")
	missingReplacementID := uint64(999999)
	orphanReplacement := newIntegrationRefreshToken(user.ID, "orphan-replacement", "family", time.Now().Add(time.Hour))
	orphanReplacement.ReplacedByID = &missingReplacementID
	if err := repository.Create(context.Background(), orphanReplacement); err == nil {
		t.Fatal("refresh_tokens.replaced_by_id accepted an unknown token; required self foreign key is missing")
	}
}

func TestRefreshTokenRepositoryIntegrationRotateSuccess(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	user := createIntegrationUser(t, users, "rotate@example.com")
	current := newIntegrationRefreshToken(user.ID, "current-hash", "family", time.Now().Add(time.Hour))
	if err := repository.Create(context.Background(), current); err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}
	now := time.Now().Truncate(time.Microsecond)
	next := newIntegrationRefreshToken(999, "next-hash", "wrong-family", now.Add(time.Hour))

	if err := repository.Rotate(context.Background(), current.TokenHash, next, now); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if next.ID == 0 || next.UserID != user.ID || next.FamilyID != current.FamilyID {
		t.Fatalf("next token = %+v", next)
	}
	storedCurrent, err := repository.FindByTokenHash(context.Background(), current.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash(current) error = %v", err)
	}
	if storedCurrent.UsedAt == nil || !storedCurrent.UsedAt.Equal(now) || storedCurrent.ReplacedByID == nil || *storedCurrent.ReplacedByID != next.ID {
		t.Fatalf("stored current token = %+v", storedCurrent)
	}
	storedNext, err := repository.FindByTokenHash(context.Background(), next.TokenHash)
	if err != nil || storedNext.UserID != user.ID || storedNext.FamilyID != current.FamilyID {
		t.Fatalf("stored next = %+v, err=%v", storedNext, err)
	}
}

func TestRefreshTokenRepositoryIntegrationRotateRejectsExpiredAndRevokedWithoutCreatingNext(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*entity.RefreshToken, time.Time)
		want      error
	}{
		{name: "expired", configure: func(token *entity.RefreshToken, now time.Time) { token.ExpiresAt = now }, want: entity.ErrRefreshTokenExpired},
		{name: "revoked", configure: func(token *entity.RefreshToken, now time.Time) {
			token.ExpiresAt = now.Add(time.Hour)
			token.RevokedAt = &now
		}, want: entity.ErrRefreshTokenRevoked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openPostgresIntegrationDB(t)
			users := NewUserRepository(db)
			repository := NewRefreshTokenRepository(db)
			user := createIntegrationUser(t, users, tt.name+"@example.com")
			now := time.Now().Truncate(time.Microsecond)
			current := newIntegrationRefreshToken(user.ID, "current-"+tt.name, "family", now.Add(time.Hour))
			tt.configure(current, now)
			if err := repository.Create(context.Background(), current); err != nil {
				t.Fatalf("Create(current) error = %v", err)
			}
			next := newIntegrationRefreshToken(user.ID, "next-"+tt.name, "family", now.Add(time.Hour))
			if err := repository.Rotate(context.Background(), current.TokenHash, next, now); !errors.Is(err, tt.want) {
				t.Fatalf("Rotate() error = %v, want %v", err, tt.want)
			}
			if _, err := repository.FindByTokenHash(context.Background(), next.TokenHash); !errors.Is(err, entity.ErrRefreshTokenInvalid) {
				t.Fatalf("next token exists after rejected rotate: %v", err)
			}
		})
	}
}

func TestRefreshTokenRepositoryIntegrationRotateRollbackKeepsOldTokenUnused(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	user := createIntegrationUser(t, users, "rollback@example.com")
	current := newIntegrationRefreshToken(user.ID, "same-hash", "family", time.Now().Add(time.Hour))
	if err := repository.Create(context.Background(), current); err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}
	next := newIntegrationRefreshToken(user.ID, "same-hash", "family", time.Now().Add(time.Hour))

	if err := repository.Rotate(context.Background(), current.TokenHash, next, time.Now()); err == nil {
		t.Fatal("Rotate() succeeded despite duplicate next token hash")
	}
	stored, err := repository.FindByTokenHash(context.Background(), current.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash() error = %v", err)
	}
	if stored.UsedAt != nil || stored.ReplacedByID != nil {
		t.Fatalf("failed rotation partially updated old token: %+v", stored)
	}
}

func TestRefreshTokenRepositoryIntegrationReuseRevokesFamilyAndInvalidatesAccessTokens(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	user := createIntegrationUser(t, users, "reuse@example.com")
	now := time.Now().Truncate(time.Microsecond)
	current := newIntegrationRefreshToken(user.ID, "current-reuse", "family-reuse", now.Add(time.Hour))
	if err := repository.Create(context.Background(), current); err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}
	next := newIntegrationRefreshToken(user.ID, "next-reuse", "family-reuse", now.Add(time.Hour))
	if err := repository.Rotate(context.Background(), current.TokenHash, next, now); err != nil {
		t.Fatalf("first Rotate() error = %v", err)
	}

	third := newIntegrationRefreshToken(user.ID, "third-reuse", "family-reuse", now.Add(time.Hour))
	if err := repository.Rotate(context.Background(), current.TokenHash, third, now.Add(time.Second)); !errors.Is(err, entity.ErrRefreshTokenReused) {
		t.Fatalf("second Rotate() error = %v, want ErrRefreshTokenReused", err)
	}
	for _, hash := range []string{current.TokenHash, next.TokenHash} {
		stored, err := repository.FindByTokenHash(context.Background(), hash)
		if err != nil {
			t.Fatalf("FindByTokenHash(%s) error = %v", hash, err)
		}
		if stored.RevokedAt == nil {
			t.Fatalf("family token %s was not revoked", hash)
		}
	}
	if _, err := repository.FindByTokenHash(context.Background(), third.TokenHash); !errors.Is(err, entity.ErrRefreshTokenInvalid) {
		t.Fatalf("third token was created during reuse handling: %v", err)
	}
	updatedUser, err := users.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if updatedUser.TokenVersion != 1 {
		t.Fatalf("TokenVersion = %d, want 1", updatedUser.TokenVersion)
	}
}

func TestRefreshTokenRepositoryIntegrationConcurrentRotateHasOneSuccessAndOneReuseDetection(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	user := createIntegrationUser(t, users, "concurrent-refresh@example.com")
	current := newIntegrationRefreshToken(user.ID, "current-concurrent", "family-concurrent", time.Now().Add(time.Hour))
	if err := repository.Create(context.Background(), current); err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			next := newIntegrationRefreshToken(user.ID, fmt.Sprintf("next-concurrent-%d", index), "family-concurrent", time.Now().Add(time.Hour))
			results <- repository.Rotate(context.Background(), current.TokenHash, next, time.Now())
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	reuses := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, entity.ErrRefreshTokenReused):
			reuses++
		default:
			t.Fatalf("unexpected Rotate() error = %v", err)
		}
	}
	if successes != 1 || reuses != 1 {
		t.Fatalf("successes=%d reuses=%d, want 1 and 1", successes, reuses)
	}
	updatedUser, err := users.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if updatedUser.TokenVersion != 1 {
		t.Fatalf("TokenVersion = %d, want 1 after concurrent reuse", updatedUser.TokenVersion)
	}
}

func TestRefreshTokenRepositoryIntegrationRevocationAndExpirationCleanup(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	userA := createIntegrationUser(t, users, "family-a@example.com")
	userB := createIntegrationUser(t, users, "family-b@example.com")
	now := time.Now().Truncate(time.Microsecond)

	tokens := []*entity.RefreshToken{
		newIntegrationRefreshToken(userA.ID, "a-1", "family-a", now.Add(time.Hour)),
		newIntegrationRefreshToken(userA.ID, "a-2", "family-a", now.Add(time.Hour)),
		newIntegrationRefreshToken(userA.ID, "a-other", "family-other", now.Add(time.Hour)),
		newIntegrationRefreshToken(userB.ID, "b-1", "family-b", now.Add(time.Hour)),
		newIntegrationRefreshToken(userB.ID, "expired", "family-expired", now.Add(-time.Second)),
	}
	for _, token := range tokens {
		if err := repository.Create(context.Background(), token); err != nil {
			t.Fatalf("Create(%s) error = %v", token.TokenHash, err)
		}
	}

	if err := repository.RevokeFamily(context.Background(), "family-a", now); err != nil {
		t.Fatalf("RevokeFamily() error = %v", err)
	}
	for _, hash := range []string{"a-1", "a-2"} {
		stored, _ := repository.FindByTokenHash(context.Background(), hash)
		if stored.RevokedAt == nil {
			t.Fatalf("%s not revoked", hash)
		}
	}
	other, _ := repository.FindByTokenHash(context.Background(), "a-other")
	if other.RevokedAt != nil {
		t.Fatal("RevokeFamily revoked a different family")
	}

	if err := repository.RevokeByUserID(context.Background(), userB.ID, now); err != nil {
		t.Fatalf("RevokeByUserID() error = %v", err)
	}
	b, _ := repository.FindByTokenHash(context.Background(), "b-1")
	if b.RevokedAt == nil {
		t.Fatal("RevokeByUserID did not revoke active token")
	}

	if err := repository.DeleteExpired(context.Background(), now); err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if _, err := repository.FindByTokenHash(context.Background(), "expired"); !errors.Is(err, entity.ErrRefreshTokenInvalid) {
		t.Fatalf("expired token still exists: %v", err)
	}
	if _, err := repository.FindByTokenHash(context.Background(), "b-1"); err != nil {
		t.Fatalf("unexpired token was deleted: %v", err)
	}
}

func TestRefreshTokenRepositoryIntegrationRollbackRestoresFamilyWhenUserUpdateFails(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	users := NewUserRepository(db)
	repository := NewRefreshTokenRepository(db)
	user := createIntegrationUser(t, users, "forced-rollback@example.com")
	now := time.Now().Truncate(time.Microsecond)
	current := newIntegrationRefreshToken(user.ID, "rollback-family-1", "rollback-family", now.Add(time.Hour))
	sibling := newIntegrationRefreshToken(user.ID, "rollback-family-2", "rollback-family", now.Add(time.Hour))
	for _, token := range []*entity.RefreshToken{current, sibling} {
		if err := repository.Create(context.Background(), token); err != nil {
			t.Fatalf("Create(%s) error = %v", token.TokenHash, err)
		}
	}

	if err := db.Exec(`CREATE FUNCTION fail_token_version_update() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'forced user update failure'; END; $$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatalf("create trigger function: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_token_version_update_trigger BEFORE UPDATE OF token_version ON users FOR EACH ROW EXECUTE FUNCTION fail_token_version_update()`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := repository.RevokeFamilyAndIncrementTokenVersion(context.Background(), user.ID, "rollback-family", now); err == nil {
		t.Fatal("RevokeFamilyAndIncrementTokenVersion() succeeded despite forced user update failure")
	}
	for _, hash := range []string{current.TokenHash, sibling.TokenHash} {
		stored, err := repository.FindByTokenHash(context.Background(), hash)
		if err != nil {
			t.Fatalf("FindByTokenHash(%s) error = %v", hash, err)
		}
		if stored.RevokedAt != nil {
			t.Fatalf("transaction rollback failed; %s remains revoked", hash)
		}
	}
	updatedUser, err := users.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if updatedUser.TokenVersion != 0 {
		t.Fatalf("TokenVersion = %d, want rollback to 0", updatedUser.TokenVersion)
	}
}
