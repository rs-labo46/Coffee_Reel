package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestUserUsecaseSignUpCreatesOnlyActiveGeneralUser(t *testing.T) {
	ctx := context.Background()
	before := time.Now()
	var created *entity.User

	users := &userRepositoryMock{
		findByEmailFunc: func(_ context.Context, email string) (*entity.User, error) {
			if email != "user@example.com" {
				t.Fatalf("FindByEmail email = %q", email)
			}
			return nil, entity.ErrUserNotFound
		},
		createFunc: func(_ context.Context, user *entity.User) error {
			created = user
			user.ID = 11
			return nil
		},
	}
	tokens := &tokenServiceMock{
		hashPasswordFunc: func(password string) (string, error) {
			if password != "  password  " {
				t.Fatalf("HashPassword password = %q", password)
			}
			return "bcrypt-hash", nil
		},
	}

	got, err := NewUserUsecase(users, &refreshTokenRepositoryMock{}, tokens).SignUp(ctx, "Alice", "user@example.com", "  password  ")
	after := time.Now()
	if err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	if got != created || got.ID != 11 {
		t.Fatalf("SignUp() user = %+v, want repository-created user", got)
	}
	if created.Name != "Alice" || created.Email != "user@example.com" || created.PasswordHash != "bcrypt-hash" {
		t.Fatalf("created user = %+v", created)
	}
	if created.Role != entity.RoleUser || created.Status != entity.StatusActive || created.TokenVersion != 0 {
		t.Fatalf("created authorization state = role:%q status:%q tv:%d", created.Role, created.Status, created.TokenVersion)
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("CreatedAt = %s, UpdatedAt = %s", created.CreatedAt, created.UpdatedAt)
	}
	assertTimeNear(t, created.CreatedAt, before, after)
}

func TestUserUsecaseSignUpStopsAtTheFailingBoundary(t *testing.T) {
	internalErr := errors.New("database unavailable")
	hashErr := errors.New("bcrypt failed")
	createErr := errors.New("insert failed")

	tests := []struct {
		name   string
		users  *userRepositoryMock
		tokens *tokenServiceMock
		want   error
	}{
		{
			name: "existing email",
			users: &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
				return &entity.User{ID: 1}, nil
			}},
			tokens: &tokenServiceMock{},
			want:   entity.ErrEmailAlreadyExists,
		},
		{
			name: "find error",
			users: &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
				return nil, internalErr
			}},
			tokens: &tokenServiceMock{},
			want:   internalErr,
		},
		{
			name: "hash error",
			users: &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
				return nil, entity.ErrUserNotFound
			}},
			tokens: &tokenServiceMock{hashPasswordFunc: func(string) (string, error) { return "", hashErr }},
			want:   hashErr,
		},
		{
			name: "repository unique race is preserved",
			users: &userRepositoryMock{
				findByEmailFunc: func(context.Context, string) (*entity.User, error) { return nil, entity.ErrUserNotFound },
				createFunc:      func(context.Context, *entity.User) error { return entity.ErrEmailAlreadyExists },
			},
			tokens: &tokenServiceMock{hashPasswordFunc: func(string) (string, error) { return "hash", nil }},
			want:   entity.ErrEmailAlreadyExists,
		},
		{
			name: "create error",
			users: &userRepositoryMock{
				findByEmailFunc: func(context.Context, string) (*entity.User, error) { return nil, entity.ErrUserNotFound },
				createFunc:      func(context.Context, *entity.User) error { return createErr },
			},
			tokens: &tokenServiceMock{hashPasswordFunc: func(string) (string, error) { return "hash", nil }},
			want:   createErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUserUsecase(tt.users, &refreshTokenRepositoryMock{}, tt.tokens).SignUp(context.Background(), "user", "user@example.com", "password")
			if !errors.Is(err, tt.want) {
				t.Fatalf("SignUp() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUserUsecaseLoginCreatesNewFamilyAndPersistsOnlyRefreshHash(t *testing.T) {
	ctx := context.Background()
	user := &entity.User{ID: 7, PasswordHash: "stored-hash", Role: entity.RoleAdmin, Status: entity.StatusActive, TokenVersion: 3}
	before := time.Now()
	var generatedAt time.Time
	var saved *entity.RefreshToken

	users := &userRepositoryMock{findByEmailFunc: func(_ context.Context, email string) (*entity.User, error) {
		if email != "admin@example.com" {
			t.Fatalf("FindByEmail email = %q", email)
		}
		return user, nil
	}}
	refreshTokens := &refreshTokenRepositoryMock{createFunc: func(_ context.Context, token *entity.RefreshToken) error {
		saved = token
		token.ID = 100
		return nil
	}}
	tokens := &tokenServiceMock{
		comparePasswordFunc: func(hash, password string) error {
			if hash != "stored-hash" || password != "password" {
				t.Fatalf("ComparePassword(%q, %q)", hash, password)
			}
			return nil
		},
		generateFamilyIDFunc: func() (string, error) { return "new-family", nil },
		generateAccessTokenFunc: func(userID uint64, role entity.UserRole, tokenVersion uint64, now time.Time) (string, error) {
			if userID != 7 || role != entity.RoleAdmin || tokenVersion != 3 {
				t.Fatalf("GenerateAccessToken(%d, %q, %d)", userID, role, tokenVersion)
			}
			generatedAt = now
			return "access-token", nil
		},
		generateRefreshTokenFunc: func() (string, error) { return "plain-refresh-token", nil },
		hashRefreshTokenFunc: func(token string) string {
			if token != "plain-refresh-token" {
				t.Fatalf("HashRefreshToken token = %q", token)
			}
			return "refresh-hash"
		},
		generateCSRFTokenFunc: func() (string, error) { return "csrf-token", nil },
	}

	result, err := NewUserUsecase(users, refreshTokens, tokens).Login(ctx, "admin@example.com", "password")
	after := time.Now()
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.User != user || result.AccessToken != "access-token" || result.RefreshToken != "plain-refresh-token" || result.CSRFToken != "csrf-token" {
		t.Fatalf("Login() result = %+v", result)
	}
	assertTimeNear(t, generatedAt, before, after)
	if saved.UserID != user.ID || saved.TokenHash != "refresh-hash" || saved.FamilyID != "new-family" {
		t.Fatalf("saved refresh token = %+v", saved)
	}
	if saved.TokenHash == result.RefreshToken {
		t.Fatal("repository received plaintext refresh token as TokenHash")
	}
	if !saved.CreatedAt.Equal(generatedAt) || !saved.ExpiresAt.Equal(generatedAt.Add(refreshTokenLifetime)) {
		t.Fatalf("saved token times = created:%s expires:%s generated:%s", saved.CreatedAt, saved.ExpiresAt, generatedAt)
	}
	if !result.RefreshTokenExpiresAt.Equal(saved.ExpiresAt) {
		t.Fatalf("response expiration = %s, want %s", result.RefreshTokenExpiresAt, saved.ExpiresAt)
	}
}

func TestUserUsecaseLoginRejectsInvalidCredentialsAndSuspendedUsersBeforeTokenIssuance(t *testing.T) {
	tests := []struct {
		name   string
		users  *userRepositoryMock
		tokens *tokenServiceMock
		want   error
	}{
		{
			name: "user not found is indistinguishable from wrong password",
			users: &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
				return nil, entity.ErrUserNotFound
			}},
			tokens: &tokenServiceMock{},
			want:   entity.ErrInvalidCredentials,
		},
		{
			name: "wrong password",
			users: &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
				return &entity.User{PasswordHash: "hash", Status: entity.StatusActive}, nil
			}},
			tokens: &tokenServiceMock{comparePasswordFunc: func(string, string) error { return entity.ErrInvalidCredentials }},
			want:   entity.ErrInvalidCredentials,
		},
		{
			name: "suspended user",
			users: &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) {
				return &entity.User{PasswordHash: "hash", Status: entity.StatusSuspended}, nil
			}},
			tokens: &tokenServiceMock{comparePasswordFunc: func(string, string) error { return nil }},
			want:   entity.ErrUserSuspended,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUserUsecase(tt.users, &refreshTokenRepositoryMock{}, tt.tokens).Login(context.Background(), "user@example.com", "password")
			if !errors.Is(err, tt.want) {
				t.Fatalf("Login() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUserUsecaseLoginPropagatesTokenAndPersistenceFailures(t *testing.T) {
	user := &entity.User{ID: 1, PasswordHash: "hash", Role: entity.RoleUser, Status: entity.StatusActive}
	users := &userRepositoryMock{findByEmailFunc: func(context.Context, string) (*entity.User, error) { return user, nil }}
	familyErr := errors.New("family random failed")
	accessErr := errors.New("jwt signing failed")
	createErr := errors.New("database insert failed")

	tests := []struct {
		name          string
		tokens        *tokenServiceMock
		refreshTokens *refreshTokenRepositoryMock
		want          error
	}{
		{
			name:          "family generation failure",
			tokens:        &tokenServiceMock{comparePasswordFunc: func(string, string) error { return nil }, generateFamilyIDFunc: func() (string, error) { return "", familyErr }},
			refreshTokens: &refreshTokenRepositoryMock{},
			want:          familyErr,
		},
		{
			name: "access token generation failure",
			tokens: &tokenServiceMock{
				comparePasswordFunc:     func(string, string) error { return nil },
				generateFamilyIDFunc:    func() (string, error) { return "family", nil },
				generateAccessTokenFunc: func(uint64, entity.UserRole, uint64, time.Time) (string, error) { return "", accessErr },
			},
			refreshTokens: &refreshTokenRepositoryMock{},
			want:          accessErr,
		},
		{
			name: "refresh token persistence failure",
			tokens: &tokenServiceMock{
				comparePasswordFunc:      func(string, string) error { return nil },
				generateFamilyIDFunc:     func() (string, error) { return "family", nil },
				generateAccessTokenFunc:  func(uint64, entity.UserRole, uint64, time.Time) (string, error) { return "access", nil },
				generateRefreshTokenFunc: func() (string, error) { return "refresh", nil },
				hashRefreshTokenFunc:     func(string) string { return "hash" },
				generateCSRFTokenFunc:    func() (string, error) { return "csrf", nil },
			},
			refreshTokens: &refreshTokenRepositoryMock{createFunc: func(context.Context, *entity.RefreshToken) error { return createErr }},
			want:          createErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUserUsecase(users, tt.refreshTokens, tt.tokens).Login(context.Background(), "user@example.com", "password")
			if !errors.Is(err, tt.want) {
				t.Fatalf("Login() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUserUsecaseRefreshRotatesWithinTheSameFamily(t *testing.T) {
	nowBefore := time.Now()
	current := &entity.RefreshToken{ID: 10, UserID: 7, TokenHash: "current-hash", FamilyID: "family", ExpiresAt: nowBefore.Add(time.Hour)}
	user := &entity.User{ID: 7, Role: entity.RoleUser, Status: entity.StatusActive, TokenVersion: 2}
	var rotateAt time.Time
	var rotated *entity.RefreshToken

	users := &userRepositoryMock{findByIDFunc: func(_ context.Context, id uint64) (*entity.User, error) {
		if id != 7 {
			t.Fatalf("FindByID id = %d", id)
		}
		return user, nil
	}}
	refreshTokens := &refreshTokenRepositoryMock{
		findByTokenHashFunc: func(_ context.Context, hash string) (*entity.RefreshToken, error) {
			if hash != "current-hash" {
				t.Fatalf("FindByTokenHash hash = %q", hash)
			}
			return current, nil
		},
		rotateFunc: func(_ context.Context, hash string, next *entity.RefreshToken, now time.Time) error {
			if hash != "current-hash" {
				t.Fatalf("Rotate hash = %q", hash)
			}
			rotateAt = now
			rotated = next
			return nil
		},
	}
	tokens := &tokenServiceMock{
		generateAccessTokenFunc: func(userID uint64, role entity.UserRole, tokenVersion uint64, now time.Time) (string, error) {
			if userID != 7 || role != entity.RoleUser || tokenVersion != 2 {
				t.Fatalf("GenerateAccessToken(%d, %q, %d)", userID, role, tokenVersion)
			}
			return "next-access", nil
		},
		generateRefreshTokenFunc: func() (string, error) { return "plain-next", nil },
		hashRefreshTokenFunc: func(token string) string {
			switch token {
			case "plain-current":
				return "current-hash"
			case "plain-next":
				return "next-hash"
			default:
				t.Fatalf("HashRefreshToken token = %q", token)
				return ""
			}
		},
		generateCSRFTokenFunc: func() (string, error) { return "next-csrf", nil },
	}

	result, err := NewUserUsecase(users, refreshTokens, tokens).Refresh(context.Background(), "plain-current")
	nowAfter := time.Now()
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if result.AccessToken != "next-access" || result.RefreshToken != "plain-next" || result.CSRFToken != "next-csrf" {
		t.Fatalf("Refresh() result = %+v", result)
	}
	assertTimeNear(t, rotateAt, nowBefore, nowAfter)
	if rotated.UserID != user.ID || rotated.FamilyID != current.FamilyID || rotated.TokenHash != "next-hash" {
		t.Fatalf("rotated token = %+v", rotated)
	}
	if !rotated.CreatedAt.Equal(rotateAt) || !rotated.ExpiresAt.Equal(rotateAt.Add(refreshTokenLifetime)) {
		t.Fatalf("rotated token times = %+v, rotateAt=%s", rotated, rotateAt)
	}
}

func TestUserUsecaseRefreshRejectsInvalidTokenStatesBeforeRotation(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(-time.Minute)
	usedAt := now.Add(-time.Minute)
	revokeCalled := false

	tests := []struct {
		name          string
		plain         string
		findToken     *entity.RefreshToken
		findErr       error
		users         *userRepositoryMock
		refreshTokens *refreshTokenRepositoryMock
		tokens        *tokenServiceMock
		want          error
	}{
		{name: "missing token", plain: "", refreshTokens: &refreshTokenRepositoryMock{}, tokens: &tokenServiceMock{}, want: entity.ErrRefreshTokenMissing},
		{
			name:  "unknown token",
			plain: "plain",
			refreshTokens: &refreshTokenRepositoryMock{findByTokenHashFunc: func(context.Context, string) (*entity.RefreshToken, error) {
				return nil, entity.ErrRefreshTokenInvalid
			}},
			tokens: &tokenServiceMock{hashRefreshTokenFunc: func(string) string { return "hash" }},
			want:   entity.ErrRefreshTokenInvalid,
		},
		{
			name:      "expired token",
			plain:     "plain",
			findToken: &entity.RefreshToken{ExpiresAt: now},
			want:      entity.ErrRefreshTokenExpired,
		},
		{
			name:      "revoked token",
			plain:     "plain",
			findToken: &entity.RefreshToken{ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt},
			want:      entity.ErrRefreshTokenRevoked,
		},
		{
			name:      "used token triggers family revocation and token version increment",
			plain:     "plain",
			findToken: &entity.RefreshToken{UserID: 9, FamilyID: "family", ExpiresAt: now.Add(time.Hour), UsedAt: &usedAt},
			refreshTokens: &refreshTokenRepositoryMock{revokeFamilyAndIncrementTokenVersionFunc: func(_ context.Context, userID uint64, familyID string, at time.Time) error {
				revokeCalled = true
				if userID != 9 || familyID != "family" {
					t.Fatalf("RevokeFamilyAndIncrementTokenVersion(%d, %q, %s)", userID, familyID, at)
				}
				return nil
			}},
			want: entity.ErrRefreshTokenReused,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tokens == nil {
				tt.tokens = &tokenServiceMock{hashRefreshTokenFunc: func(string) string { return "hash" }}
			}
			if tt.refreshTokens == nil {
				tt.refreshTokens = &refreshTokenRepositoryMock{}
			}
			if tt.findToken != nil {
				tt.refreshTokens.findByTokenHashFunc = func(context.Context, string) (*entity.RefreshToken, error) { return tt.findToken, nil }
			}
			if tt.users == nil {
				tt.users = &userRepositoryMock{}
			}
			_, err := NewUserUsecase(tt.users, tt.refreshTokens, tt.tokens).Refresh(context.Background(), tt.plain)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Refresh() error = %v, want %v", err, tt.want)
			}
		})
	}
	if !revokeCalled {
		t.Fatal("used token did not revoke the family and increment TokenVersion")
	}
}

func TestUserUsecaseRefreshRejectsSuspendedUserAndPropagatesRotateRace(t *testing.T) {
	now := time.Now()
	current := &entity.RefreshToken{UserID: 1, FamilyID: "family", ExpiresAt: now.Add(time.Hour)}
	baseTokens := func() *tokenServiceMock {
		return &tokenServiceMock{
			hashRefreshTokenFunc:     func(string) string { return "hash" },
			generateAccessTokenFunc:  func(uint64, entity.UserRole, uint64, time.Time) (string, error) { return "access", nil },
			generateRefreshTokenFunc: func() (string, error) { return "refresh", nil },
			generateCSRFTokenFunc:    func() (string, error) { return "csrf", nil },
		}
	}

	t.Run("suspended user", func(t *testing.T) {
		repo := &refreshTokenRepositoryMock{findByTokenHashFunc: func(context.Context, string) (*entity.RefreshToken, error) { return current, nil }}
		users := &userRepositoryMock{findByIDFunc: func(context.Context, uint64) (*entity.User, error) {
			return &entity.User{ID: 1, Status: entity.StatusSuspended}, nil
		}}
		_, err := NewUserUsecase(users, repo, baseTokens()).Refresh(context.Background(), "plain")
		if !errors.Is(err, entity.ErrUserSuspended) {
			t.Fatalf("Refresh() error = %v, want ErrUserSuspended", err)
		}
	})

	t.Run("repository detects concurrent reuse", func(t *testing.T) {
		repo := &refreshTokenRepositoryMock{
			findByTokenHashFunc: func(context.Context, string) (*entity.RefreshToken, error) { return current, nil },
			rotateFunc: func(context.Context, string, *entity.RefreshToken, time.Time) error {
				return entity.ErrRefreshTokenReused
			},
		}
		users := &userRepositoryMock{findByIDFunc: func(context.Context, uint64) (*entity.User, error) {
			return &entity.User{ID: 1, Role: entity.RoleUser, Status: entity.StatusActive}, nil
		}}
		tokens := baseTokens()
		tokens.hashRefreshTokenFunc = func(token string) string {
			if token == "refresh" {
				return "next-hash"
			}
			return "hash"
		}
		_, err := NewUserUsecase(users, repo, tokens).Refresh(context.Background(), "plain")
		if !errors.Is(err, entity.ErrRefreshTokenReused) {
			t.Fatalf("Refresh() error = %v, want ErrRefreshTokenReused", err)
		}
	})
}

func TestUserUsecaseLogoutIsIdempotentForMissingAndAlreadyRevokedTokens(t *testing.T) {
	t.Run("missing cookie is successful", func(t *testing.T) {
		err := NewUserUsecase(&userRepositoryMock{}, &refreshTokenRepositoryMock{}, &tokenServiceMock{}).Logout(context.Background(), "")
		if err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
	})

	t.Run("already revoked stored token is successful", func(t *testing.T) {
		revokedAt := time.Now().Add(-time.Minute)
		revokeCalls := 0
		repo := &refreshTokenRepositoryMock{
			findByTokenHashFunc: func(_ context.Context, hash string) (*entity.RefreshToken, error) {
				if hash != "hash" {
					t.Fatalf("FindByTokenHash hash = %q", hash)
				}
				return &entity.RefreshToken{FamilyID: "family", RevokedAt: &revokedAt}, nil
			},
			revokeFamilyFunc: func(_ context.Context, familyID string, now time.Time) error {
				revokeCalls++
				if familyID != "family" {
					t.Fatalf("RevokeFamily(%q, %s)", familyID, now)
				}
				return nil
			},
		}
		tokens := &tokenServiceMock{hashRefreshTokenFunc: func(token string) string {
			if token != "plain" {
				t.Fatalf("HashRefreshToken token = %q", token)
			}
			return "hash"
		}}
		uc := NewUserUsecase(&userRepositoryMock{}, repo, tokens)
		if err := uc.Logout(context.Background(), "plain"); err != nil {
			t.Fatalf("first Logout() error = %v", err)
		}
		if err := uc.Logout(context.Background(), "plain"); err != nil {
			t.Fatalf("second Logout() error = %v", err)
		}
		if revokeCalls != 2 {
			t.Fatalf("RevokeFamily calls = %d, want 2 idempotent calls", revokeCalls)
		}
	})
}

func TestUserUsecaseGetMeAndValidateTokenVersion(t *testing.T) {
	t.Run("zero ID", func(t *testing.T) {
		_, err := NewUserUsecase(&userRepositoryMock{}, &refreshTokenRepositoryMock{}, &tokenServiceMock{}).GetMe(context.Background(), 0)
		if !errors.Is(err, entity.ErrUnauthorized) {
			t.Fatalf("GetMe() error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("missing user is unauthorized", func(t *testing.T) {
		users := &userRepositoryMock{findByIDFunc: func(context.Context, uint64) (*entity.User, error) { return nil, entity.ErrUserNotFound }}
		_, err := NewUserUsecase(users, &refreshTokenRepositoryMock{}, &tokenServiceMock{}).GetMe(context.Background(), 1)
		if !errors.Is(err, entity.ErrUnauthorized) {
			t.Fatalf("GetMe() error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("suspended user", func(t *testing.T) {
		users := &userRepositoryMock{findByIDFunc: func(context.Context, uint64) (*entity.User, error) {
			return &entity.User{ID: 1, Status: entity.StatusSuspended}, nil
		}}
		_, err := NewUserUsecase(users, &refreshTokenRepositoryMock{}, &tokenServiceMock{}).GetMe(context.Background(), 1)
		if !errors.Is(err, entity.ErrUserSuspended) {
			t.Fatalf("GetMe() error = %v, want ErrUserSuspended", err)
		}
	})

	t.Run("active user and token version", func(t *testing.T) {
		user := &entity.User{ID: 1, Status: entity.StatusActive, TokenVersion: 5}
		users := &userRepositoryMock{findByIDFunc: func(context.Context, uint64) (*entity.User, error) { return user, nil }}
		uc := NewUserUsecase(users, &refreshTokenRepositoryMock{}, &tokenServiceMock{})
		got, err := uc.GetMe(context.Background(), 1)
		if err != nil || got != user {
			t.Fatalf("GetMe() = (%+v, %v)", got, err)
		}
		if err := uc.ValidateTokenVersion(user, 5); err != nil {
			t.Fatalf("ValidateTokenVersion(match) error = %v", err)
		}
		if err := uc.ValidateTokenVersion(user, 4); !errors.Is(err, entity.ErrUnauthorized) {
			t.Fatalf("ValidateTokenVersion(mismatch) error = %v", err)
		}
		if err := uc.ValidateTokenVersion(nil, 5); !errors.Is(err, entity.ErrUnauthorized) {
			t.Fatalf("ValidateTokenVersion(nil) error = %v", err)
		}
	})
}
