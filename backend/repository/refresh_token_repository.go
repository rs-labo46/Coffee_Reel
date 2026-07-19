type IRefreshTokenRepository interface {
	Create(ctx context.Context, token *entity.RefreshToken) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*entity.RefreshToken, error)
	Rotate(ctx context.Context, ctx context.Context, tokenHash string, nextToken *entity.RefreshToken, now time.Time) error
	RevokeFamilly(ctx context.Context, familyID string, now time.Time) error
	RevokeFamillyAndIncrementTokenVersion(ctx context.Context, userID uint64, famillyID string, now time.Time) error
	RevokeByUserID(ctx context.Context, userID uint64, now time.Time) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

type refreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) IRefreshTokenRepository {
	return &refreshTokenRepository{db}
}

// リフレッシュトークン作成
func (r *refreshTokenRepository) Create(ctx context.Context, token *entity.RefreshToken) error {
	if token == nil {
		return fmt.Errorf("refresh token is required")
	}

	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

// ハッシュからリフレッシュトークンを取得
func (r *refreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*entity.RefreshToken, error) {
	if tokenHash == "" {
		return nil, entity.ErrRefreshTokenInvalid
	}
	var token entity.RefreshToken

	if err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entity.ErrRefreshTokenInvalid
		}
		return nil, fmt.Errorf("find refresh token by hash: %w", err)
	}
	return &token, nil
}

// 古いtokenを取得してロックし、新しいtoken作成,tokenの状態を使用済みに
func (r *refreshTokenRepository) Rotate(ctx context.Context, tokenHash string, nextToken *entity.RefreshToken, now time.Time) error {
	if tokenHash == "" {
		return entity.ErrRefreshTokenInvalid
	}

	if nextToken == nil {
		return fmt.Errorf("next refresh token is required")
	}

	now = now.UTC()
	reused := false

	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var currentToken entity.RefreshToken

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).First(&currentToken).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrRefreshTokenInvalid
			}

			return fmt.Errorf("lock refresh token: %w", err)
		}

		switch {
		case currentToken.IsExpired(now):
			return entity.ErrRefreshTokenExpired

		case currentToken.RevokedAt != nil:
			return entity.ErrRefreshTokenRevoked

		case currentToken.UsedAt != nil:
			if err := revokeFamilyAndIncrementTokenVersion(tx, currentToken.UserID, currentToken.FamilyID, now); err != nil {
				return err
			}

			reused = true
			return nil
		}

		nextToken.UserID = currentToken.UserID
		nextToken.FamilyID = currentToken.FamilyID

		if err := tx.Create(nextToken).Error; err != nil {
			return fmt.Errorf("create rotated refresh token: %w", err)
		}

		currentToken.MarkUsed(nextToken.ID, now)

		result := tx.Model(&entity.RefreshToken{}).Where("id = ?", currentToken.ID).Select("used_at", "replaced_by_id").Updates(&currentToken)

		if result.Error != nil {
			return fmt.Errorf("mark refresh token used: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("mark refresh token used: token not found")
		}

		return nil

	}); err != nil {
		return err
	}

	if reused {
		return entity.ErrRefreshTokenReused
	}
	return nil
}

// 同じFamilyIDのTokenを一括失効する
func (r *refreshTokenRepository) RevokeFamilly(ctx context.Context, familyID string, now time.Time) error {
	if famillyID == "" {
		return fmt.Errorf("family ID is required")
	}
	if err := r.db.WithContext(ctx).Model(&entity.RefreshToken{}).Where("family_id = ? AND revoked_at IS NULL", familyID).Update("revoked_at", now.UTC()).Error; err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}

//再利用検知時にFamily失効とUser.TokenVersion更新を同じTransactionで行う

//Userに属するRefresh Tokenを一括失効する

//期限切れのTokenを削除する