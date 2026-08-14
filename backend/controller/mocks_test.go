package controller

import (
	"context"

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
}

func (m *userUsecaseMock) SignUp(ctx context.Context, name, email, password string) (*entity.User, error) {
	if m.signUpFunc == nil {
		panic("unexpected UserUsecase.SignUp call")
	}
	return m.signUpFunc(ctx, name, email, password)
}
func (m *userUsecaseMock) Login(ctx context.Context, email, password string) (usecase.LoginResult, error) {
	if m.loginFunc == nil {
		panic("unexpected UserUsecase.Login call")
	}
	return m.loginFunc(ctx, email, password)
}
func (m *userUsecaseMock) Refresh(ctx context.Context, token string) (usecase.RefreshResult, error) {
	if m.refreshFunc == nil {
		panic("unexpected UserUsecase.Refresh call")
	}
	return m.refreshFunc(ctx, token)
}
func (m *userUsecaseMock) Logout(ctx context.Context, token string) error {
	if m.logoutFunc == nil {
		panic("unexpected UserUsecase.Logout call")
	}
	return m.logoutFunc(ctx, token)
}
func (m *userUsecaseMock) GetMe(ctx context.Context, userID uint64) (*entity.User, error) {
	if m.getMeFunc == nil {
		panic("unexpected UserUsecase.GetMe call")
	}
	return m.getMeFunc(ctx, userID)
}
func (m *userUsecaseMock) ValidateTokenVersion(user *entity.User, tokenVersion uint64) error {
	if m.validateTokenVersionFunc == nil {
		panic("unexpected UserUsecase.ValidateTokenVersion call")
	}
	return m.validateTokenVersionFunc(user, tokenVersion)
}

type rateLimitUsecaseMock struct {
	allowSignupFunc            func(context.Context, string) (usecase.RateLimitDecision, error)
	allowLoginIPFunc           func(context.Context, string) (usecase.RateLimitDecision, error)
	allowLoginEmailFunc        func(context.Context, string) (usecase.RateLimitDecision, error)
	allowRefreshFunc           func(context.Context, string) (usecase.RateLimitDecision, error)
	allowVideoStartUserFunc    func(context.Context, uint64) (usecase.RateLimitDecision, error)
	allowVideoStartIPFunc      func(context.Context, string) (usecase.RateLimitDecision, error)
	allowVideoCompleteUserFunc func(context.Context, uint64) (usecase.RateLimitDecision, error)
	allowVideoCompleteIPFunc   func(context.Context, string) (usecase.RateLimitDecision, error)
}

func (m *rateLimitUsecaseMock) AllowSignup(ctx context.Context, ip string) (usecase.RateLimitDecision, error) {
	if m.allowSignupFunc == nil {
		panic("unexpected RateLimitUsecase.AllowSignup call")
	}
	return m.allowSignupFunc(ctx, ip)
}
func (m *rateLimitUsecaseMock) AllowLoginIP(ctx context.Context, ip string) (usecase.RateLimitDecision, error) {
	if m.allowLoginIPFunc == nil {
		panic("unexpected RateLimitUsecase.AllowLoginIP call")
	}
	return m.allowLoginIPFunc(ctx, ip)
}
func (m *rateLimitUsecaseMock) AllowLoginEmail(ctx context.Context, email string) (usecase.RateLimitDecision, error) {
	if m.allowLoginEmailFunc == nil {
		panic("unexpected RateLimitUsecase.AllowLoginEmail call")
	}
	return m.allowLoginEmailFunc(ctx, email)
}
func (m *rateLimitUsecaseMock) AllowRefresh(ctx context.Context, token string) (usecase.RateLimitDecision, error) {
	if m.allowRefreshFunc == nil {
		panic("unexpected RateLimitUsecase.AllowRefresh call")
	}
	return m.allowRefreshFunc(ctx, token)
}
func (m *rateLimitUsecaseMock) AllowVideoStartUser(ctx context.Context, userID uint64) (usecase.RateLimitDecision, error) {
	if m.allowVideoStartUserFunc == nil {
		panic("unexpected RateLimitUsecase.AllowVideoStartUser call")
	}
	return m.allowVideoStartUserFunc(ctx, userID)
}
func (m *rateLimitUsecaseMock) AllowVideoStartIP(ctx context.Context, ip string) (usecase.RateLimitDecision, error) {
	if m.allowVideoStartIPFunc == nil {
		panic("unexpected RateLimitUsecase.AllowVideoStartIP call")
	}
	return m.allowVideoStartIPFunc(ctx, ip)
}
func (m *rateLimitUsecaseMock) AllowVideoCompleteUser(ctx context.Context, userID uint64) (usecase.RateLimitDecision, error) {
	if m.allowVideoCompleteUserFunc == nil {
		panic("unexpected RateLimitUsecase.AllowVideoCompleteUser call")
	}
	return m.allowVideoCompleteUserFunc(ctx, userID)
}
func (m *rateLimitUsecaseMock) AllowVideoCompleteIP(ctx context.Context, ip string) (usecase.RateLimitDecision, error) {
	if m.allowVideoCompleteIPFunc == nil {
		panic("unexpected RateLimitUsecase.AllowVideoCompleteIP call")
	}
	return m.allowVideoCompleteIPFunc(ctx, ip)
}

type userValidatorMock struct {
	validateSignupFunc func(string, string, string) (string, string, string, error)
	validateLoginFunc  func(string, string) (string, string, error)
}

func (m *userValidatorMock) ValidateSignup(name, email, password string) (string, string, string, error) {
	if m.validateSignupFunc == nil {
		panic("unexpected UserValidator.ValidateSignup call")
	}
	return m.validateSignupFunc(name, email, password)
}
func (m *userValidatorMock) ValidateLogin(email, password string) (string, string, error) {
	if m.validateLoginFunc == nil {
		panic("unexpected UserValidator.ValidateLogin call")
	}
	return m.validateLoginFunc(email, password)
}
