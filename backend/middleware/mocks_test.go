package middleware

import (
	"context"
	"time"

	"coffee-reel/entity"
	"coffee-reel/usecase"
)

type userUsecaseMock struct {
	signUpFunc               func(context.Context, string, string, string) (*entity.User, error)
	loginFunc                func(context.Context, string, string) (usecase.LoginResult, error)
	refreshFunc              func(context.Context, string) (usecase.RefreshResult, error)
	logoutFunc               func(context.Context, string) error
	getMeFunc                func(context.Context, uint64) (*entity.User, error)
	validateTokenVersionFunc func(*entity.User, uint64) error
	issueCSRFTokenFunc       func() (usecase.CSRFTokenResult, error)
}

func (m *userUsecaseMock) SignUp(
	ctx context.Context,
	name,
	email,
	password string,
) (*entity.User, error) {
	if m.signUpFunc == nil {
		panic("unexpected UserUsecase.SignUp call")
	}

	return m.signUpFunc(ctx, name, email, password)
}

func (m *userUsecaseMock) Login(
	ctx context.Context,
	email,
	password string,
) (usecase.LoginResult, error) {
	if m.loginFunc == nil {
		panic("unexpected UserUsecase.Login call")
	}

	return m.loginFunc(ctx, email, password)
}

func (m *userUsecaseMock) Refresh(
	ctx context.Context,
	token string,
) (usecase.RefreshResult, error) {
	if m.refreshFunc == nil {
		panic("unexpected UserUsecase.Refresh call")
	}

	return m.refreshFunc(ctx, token)
}

func (m *userUsecaseMock) Logout(
	ctx context.Context,
	token string,
) error {
	if m.logoutFunc == nil {
		panic("unexpected UserUsecase.Logout call")
	}

	return m.logoutFunc(ctx, token)
}

func (m *userUsecaseMock) GetMe(
	ctx context.Context,
	userID uint64,
) (*entity.User, error) {
	if m.getMeFunc == nil {
		panic("unexpected UserUsecase.GetMe call")
	}

	return m.getMeFunc(ctx, userID)
}

func (m *userUsecaseMock) ValidateTokenVersion(
	user *entity.User,
	tokenVersion uint64,
) error {
	if m.validateTokenVersionFunc == nil {
		panic("unexpected UserUsecase.ValidateTokenVersion call")
	}

	return m.validateTokenVersionFunc(
		user,
		tokenVersion,
	)
}

func (m *userUsecaseMock) IssueCSRFToken() (usecase.CSRFTokenResult, error) {
	if m.issueCSRFTokenFunc == nil {
		panic("unexpected UserUsecase.IssueCSRFToken call")
	}

	return m.issueCSRFTokenFunc()
}

type tokenServiceMock struct {
	parseAccessTokenFunc func(
		string,
		time.Time,
	) (usecase.AccessTokenClaims, error)

	compareCSRFTokenFunc func(
		string,
		string,
	) error
}

func (m *tokenServiceMock) HashPassword(
	string,
) (string, error) {
	panic(
		"unexpected TokenService.HashPassword call",
	)
}

func (m *tokenServiceMock) ComparePassword(
	string,
	string,
) error {
	panic(
		"unexpected TokenService.ComparePassword call",
	)
}

func (m *tokenServiceMock) GenerateAccessToken(
	uint64,
	entity.UserRole,
	uint64,
	time.Time,
) (string, error) {
	panic(
		"unexpected TokenService.GenerateAccessToken call",
	)
}

func (m *tokenServiceMock) ParseAccessToken(
	token string,
	now time.Time,
) (usecase.AccessTokenClaims, error) {
	if m.parseAccessTokenFunc == nil {
		panic(
			"unexpected TokenService.ParseAccessToken call",
		)
	}

	return m.parseAccessTokenFunc(
		token,
		now,
	)
}

func (m *tokenServiceMock) GenerateRefreshToken() (
	string,
	error,
) {
	panic(
		"unexpected TokenService.GenerateRefreshToken call",
	)
}

func (m *tokenServiceMock) HashRefreshToken(
	string,
) string {
	panic(
		"unexpected TokenService.HashRefreshToken call",
	)
}

func (m *tokenServiceMock) GenerateFamilyID() (
	string,
	error,
) {
	panic(
		"unexpected TokenService.GenerateFamilyID call",
	)
}

func (m *tokenServiceMock) GenerateCSRFToken() (
	string,
	error,
) {
	panic(
		"unexpected TokenService.GenerateCSRFToken call",
	)
}

func (m *tokenServiceMock) CompareCSRFToken(
	cookieToken,
	headerToken string,
) error {
	if m.compareCSRFTokenFunc == nil {
		panic(
			"unexpected TokenService.CompareCSRFToken call",
		)
	}

	return m.compareCSRFTokenFunc(
		cookieToken,
		headerToken,
	)
}

type rateLimitUsecaseMock struct {
	allowSignupFunc func(
		context.Context,
		string,
	) (usecase.RateLimitDecision, error)

	allowLoginIPFunc func(
		context.Context,
		string,
	) (usecase.RateLimitDecision, error)

	allowLoginEmailFunc func(
		context.Context,
		string,
	) (usecase.RateLimitDecision, error)

	allowRefreshFunc func(
		context.Context,
		string,
	) (usecase.RateLimitDecision, error)

	allowVideoStartUserFunc func(
		context.Context,
		uint64,
	) (usecase.RateLimitDecision, error)

	allowVideoStartIPFunc func(
		context.Context,
		string,
	) (usecase.RateLimitDecision, error)

	allowVideoCompleteUserFunc func(
		context.Context,
		uint64,
	) (usecase.RateLimitDecision, error)

	allowVideoCompleteIPFunc func(
		context.Context,
		string,
	) (usecase.RateLimitDecision, error)
}

func (m *rateLimitUsecaseMock) AllowSignup(
	ctx context.Context,
	clientIP string,
) (usecase.RateLimitDecision, error) {
	if m.allowSignupFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowSignup call",
		)
	}

	return m.allowSignupFunc(
		ctx,
		clientIP,
	)
}

func (m *rateLimitUsecaseMock) AllowLoginIP(
	ctx context.Context,
	clientIP string,
) (usecase.RateLimitDecision, error) {
	if m.allowLoginIPFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowLoginIP call",
		)
	}

	return m.allowLoginIPFunc(
		ctx,
		clientIP,
	)
}

func (m *rateLimitUsecaseMock) AllowLoginEmail(
	ctx context.Context,
	email string,
) (usecase.RateLimitDecision, error) {
	if m.allowLoginEmailFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowLoginEmail call",
		)
	}

	return m.allowLoginEmailFunc(
		ctx,
		email,
	)
}

func (m *rateLimitUsecaseMock) AllowRefresh(
	ctx context.Context,
	token string,
) (usecase.RateLimitDecision, error) {
	if m.allowRefreshFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowRefresh call",
		)
	}

	return m.allowRefreshFunc(
		ctx,
		token,
	)
}

func (m *rateLimitUsecaseMock) AllowVideoStartUser(
	ctx context.Context,
	userID uint64,
) (usecase.RateLimitDecision, error) {
	if m.allowVideoStartUserFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowVideoStartUser call",
		)
	}

	return m.allowVideoStartUserFunc(
		ctx,
		userID,
	)
}

func (m *rateLimitUsecaseMock) AllowVideoStartIP(
	ctx context.Context,
	clientIP string,
) (usecase.RateLimitDecision, error) {
	if m.allowVideoStartIPFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowVideoStartIP call",
		)
	}

	return m.allowVideoStartIPFunc(
		ctx,
		clientIP,
	)
}

func (m *rateLimitUsecaseMock) AllowVideoCompleteUser(
	ctx context.Context,
	userID uint64,
) (usecase.RateLimitDecision, error) {
	if m.allowVideoCompleteUserFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowVideoCompleteUser call",
		)
	}

	return m.allowVideoCompleteUserFunc(
		ctx,
		userID,
	)
}

func (m *rateLimitUsecaseMock) AllowVideoCompleteIP(
	ctx context.Context,
	clientIP string,
) (usecase.RateLimitDecision, error) {
	if m.allowVideoCompleteIPFunc == nil {
		panic(
			"unexpected RateLimitUsecase.AllowVideoCompleteIP call",
		)
	}

	return m.allowVideoCompleteIPFunc(
		ctx,
		clientIP,
	)
}
