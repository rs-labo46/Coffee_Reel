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

func newIntegrationUser(email string) *entity.User {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &entity.User{
		Name:         "Integration User",
		Email:        email,
		PasswordHash: "$2a$10$integration-only-hash",
		Role:         entity.RoleUser,
		Status:       entity.StatusActive,
		TokenVersion: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestUserRepositoryIntegrationCreateFindAndUpdate(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewUserRepository(db)
	ctx := context.Background()
	user := newIntegrationUser("user@example.com")

	if err := repository.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.ID == 0 {
		t.Fatal("Create() did not assign an ID")
	}

	byEmail, err := repository.FindByEmail(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v", err)
	}
	byID, err := repository.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if byEmail.ID != user.ID || byID.Email != user.Email || byID.PasswordHash != user.PasswordHash {
		t.Fatalf("loaded users = byEmail:%+v byID:%+v", byEmail, byID)
	}

	originalName := byID.Name
	originalEmail := byID.Email
	originalHash := byID.PasswordHash
	updatedAt := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)
	byID.Name = "must not be updated"
	byID.Email = "must-not-change@example.com"
	byID.PasswordHash = "must-not-change"
	byID.Status = entity.StatusSuspended
	byID.TokenVersion = 3
	byID.UpdatedAt = updatedAt

	if err := repository.Update(ctx, byID); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updated, err := repository.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID(updated) error = %v", err)
	}
	if updated.Status != entity.StatusSuspended || updated.TokenVersion != 3 || !updated.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated authorization state = %+v", updated)
	}
	if updated.Name != originalName || updated.Email != originalEmail || updated.PasswordHash != originalHash {
		t.Fatalf("Update changed fields outside its responsibility: %+v", updated)
	}
}

func TestUserRepositoryIntegrationMapsMissingAndDuplicateRowsToDomainErrors(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewUserRepository(db)
	ctx := context.Background()

	if _, err := repository.FindByEmail(ctx, "missing@example.com"); !errors.Is(err, entity.ErrUserNotFound) {
		t.Fatalf("FindByEmail(missing) error = %v, want ErrUserNotFound", err)
	}
	if _, err := repository.FindByID(ctx, 999999); !errors.Is(err, entity.ErrUserNotFound) {
		t.Fatalf("FindByID(missing) error = %v, want ErrUserNotFound", err)
	}
	missing := newIntegrationUser("missing-update@example.com")
	missing.ID = 999999
	if err := repository.Update(ctx, missing); !errors.Is(err, entity.ErrUserNotFound) {
		t.Fatalf("Update(missing) error = %v, want ErrUserNotFound", err)
	}

	first := newIntegrationUser("duplicate@example.com")
	if err := repository.Create(ctx, first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second := newIntegrationUser("duplicate@example.com")
	if err := repository.Create(ctx, second); !errors.Is(err, entity.ErrEmailAlreadyExists) {
		t.Fatalf("Create(duplicate) error = %v, want ErrEmailAlreadyExists", err)
	}
}

func TestUserRepositoryIntegrationConcurrentDuplicateEmailHasOneWinner(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repository := NewUserRepository(db)
	ctx := context.Background()
	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			user := newIntegrationUser("race@example.com")
			user.Name = fmt.Sprintf("user-%d", index)
			errorsCh <- repository.Create(ctx, user)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errorsCh)

	successes := 0
	duplicates := 0
	for err := range errorsCh {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, entity.ErrEmailAlreadyExists):
			duplicates++
		default:
			t.Fatalf("unexpected Create() error = %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("successes=%d duplicates=%d, want 1 and 1", successes, duplicates)
	}
}
