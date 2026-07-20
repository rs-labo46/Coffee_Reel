package controller

import (
	"coffee-reel/entity"
	"coffee-reel/usecase"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type IUserController interface {
	SignUp(c echo.Context) error
	Login(c echo.Context) error
	Refresh(c echo.Context) error
	Logout(c echo.Context) error
	Me(c echo.Context) error
}

//interfaceのValidatorのち追記

type CookieConfig struct {
	Secure     bool
	CSRFDomain string
}

type userController struct {
	users usecase.IUserUsecase
	//のちにvalidator
	cookies CookieConfig
}

type signUpRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        uint64            `json:"id"`
	Name      string            `json:"name"`
	Email     string            `json:"email"`
	Role      entity.UserRole   `json:"role"`
	Status    entity.UserStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
}

type authUserResponse struct {
	ID     uint64            `json:"id"`
	Name   string            `json:"name"`
	Email  string            `json:"email"`
	Role   entity.UserRole   `json:"role"`
	Status entity.UserStatus `json:"status"`
}

type userDataResponse struct {
	Data userResponse `json:"data"`
}

type authResponse struct {
	Data authData `json:"data"`
}

type authData struct {
	AccessToken string           `json:"access_token"`
	TokenType   string           `json:"token_type"`
	ExpiresIn   int              `json:"expires_in"`
	User        authUserResponse `json:"user"`
}

type refreshResponse struct {
	Data refreshData `json:"data"`
}

type refreshData struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type apiErrorResponse struct {
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func NewUserController(users usecase.IUserUsecase, cookies CookieConfig) IUserController {
	return &userController{
		users:   users,
		cookies: cookies,
	}
}

const (
	refreshCookieName    = "refresh_token"
	csrfCookieName       = "csrf_token"
	cookiePath           = "/"
	cookieMaxAge         = 7 * 24 * 60 * 60
	accessTokenExpiresIn = 15 * 60
	userContextKey       = "user"
)

// 会員登録Requestを受け取り、UserUsecaseへ登録を依頼して作成したUserを返す。
func (u *userController) SignUp(c echo.Context) error {
	var req signUpRequest

	if err := c.Bind(&req); err != nil {
		return writeError(c, entity.ErrInvalidInput)
	}

	//validator実装後追記

	user, err := u.users.SignUp(c.Request().Context(), req.Name, req.Email, req.Password)
	if err != nil {
		return writeError(c, err)
	}

	return c.JSON(http.StatusCreated, userDataResponse{
		Data: newUserResponse(user),
	})
}

// Login Requestを受け取り、認証成功時に認証CookieとAccess Token、User情報を返す。
func (u *userController) Login(c echo.Context) error {
	var req loginRequest

	if err := c.Bind(&req); err != nil {
		return writeError(c, entity.ErrInvalidInput)
	}

	//validator実装後追加

	result, err := u.users.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return writeError(c, err)
	}

	u.setAuthCookies(c, result.AuthTokens)

	return c.JSON(http.StatusOK, authResponse{
		Data: authData{AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: accessTokenExpiresIn, User: newAuthUserResponse(result.User)},
	})
}

// Refresh Token Cookieを使用してTokenを再発行し、認証CookieとAccess Tokenを更新する。
func (u *userController) Refresh(c echo.Context) error {
	cookie, err := c.Cookie(refreshCookieName)
	if err != nil {
		return writeError(c, entity.ErrRefreshTokenMissing)
	}

	tokens, err := u.users.Refresh(c.Request().Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, entity.ErrRefreshTokenReused) {
			u.clearAuthCookies(c)
		}
		return writeError(c, err)
	}

	u.setAuthCookies(c, tokens)

	return c.JSON(http.StatusOK, refreshResponse{
		Data: refreshData{AccessToken: tokens.AccessToken, TokenType: "Bearer", ExpiresIn: accessTokenExpiresIn},
	})
}

// Refresh Token系列を失効し、Refresh Token CookieとCSRF Cookieを削除する。
func (u *userController) Logout(c echo.Context) error {
	plainRefreshToken := ""

	if cookie, err := c.Cookie(refreshCookieName); err == nil {
		plainRefreshToken = cookie.Value
	}

	if err := u.users.Logout(c.Request().Context(), plainRefreshToken); err != nil {
		return writeError(c, err)
	}

	u.clearAuthCookies(c)

	return c.NoContent(http.StatusNoContent)
}

// Auth MiddlewareがContextへ保存した認証済みUserを返す。
func (u *userController) Me(c echo.Context) error {
	user, ok := c.Get(userContextKey).(*entity.User)

	if !ok || user == nil {
		return writeError(c, entity.ErrUnauthorized)
	}

	return c.JSON(http.StatusOK, userDataResponse{Data: newUserResponse(user)})
}

// LoginまたはRefresh成功時に、Refresh Token CookieとCSRF Cookieを設定する。
func (u *userController) setAuthCookies(c echo.Context, tokens usecase.AuthTokens) {
	c.SetCookie(&http.Cookie{Name: refreshCookieName, Value: tokens.RefreshToken, Path: cookiePath, Expires: tokens.RefreshTokenExpiresAt.UTC(), MaxAge: cookieMaxAge, Secure: u.cookies.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	c.SetCookie(&http.Cookie{Name: csrfCookieName, Value: tokens.CSRFToken, Path: cookiePath, Domain: u.cookies.CSRFDomain, Expires: tokens.RefreshTokenExpiresAt.UTC(), MaxAge: cookieMaxAge, Secure: u.cookies.Secure, HttpOnly: false, SameSite: http.SameSiteLaxMode})
}

// 認証Cookieを発行時と同じ属性で期限切れにし、Browserから削除する。
func (u *userController) clearAuthCookies(c echo.Context) {
	expiresAt := time.Unix(0, 0).UTC()

	c.SetCookie(&http.Cookie{Name: refreshCookieName, Value: "", Path: cookiePath, Expires: expiresAt, MaxAge: -1, Secure: u.cookies.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	c.SetCookie(&http.Cookie{Name: csrfCookieName, Value: "", Path: cookiePath, Domain: u.cookies.CSRFDomain, Expires: expiresAt, MaxAge: -1, Secure: u.cookies.Secure, HttpOnly: false, SameSite: http.SameSiteLaxMode})
}

// Domain Errorを利用者へ返す。HTTP Statusと共通API Errorへ変換する。
func writeError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, entity.ErrInvalidInput):
		return writeAPIError(c, http.StatusBadRequest, "validation_failed", "入力内容が正しくありません")

	case errors.Is(err, entity.ErrEmailAlreadyExists):
		return writeAPIError(c, http.StatusConflict, "email_already_exists", "このメールアドレスは既に登録されています")

	case errors.Is(err, entity.ErrInvalidCredentials):
		return writeAPIError(c, http.StatusUnauthorized, "invalid_credentials", "メールアドレスまたはパスワードが正しくありません")

	case errors.Is(err, entity.ErrUserSuspended):
		return writeAPIError(c, http.StatusForbidden, "user_suspended", "このアカウントは利用停止中です")

	case errors.Is(err, entity.ErrRefreshTokenMissing),
		errors.Is(err, entity.ErrRefreshTokenInvalid),
		errors.Is(err, entity.ErrRefreshTokenExpired),
		errors.Is(err, entity.ErrRefreshTokenRevoked),
		errors.Is(err, entity.ErrRefreshTokenReused),
		errors.Is(err, entity.ErrUserNotFound),
		errors.Is(err, entity.ErrUnauthorized):

		return writeAPIError(c, http.StatusUnauthorized, "unauthorized", "認証情報が無効です")

	default:
		return writeAPIError(c, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました")
	}
}

// HTTP Status、Error Code、Message、Request IDを共通エラー形式で返す。
func writeAPIError(c echo.Context, status int, code string, message string) error {
	return c.JSON(status, apiErrorResponse{Status: status, Code: code, Message: message, RequestID: requestID(c)})
}

// ResponseまたはRequest HeaderからRequest IDを取得する。
func requestID(c echo.Context) string {
	if id := c.Response().Header().Get(echo.HeaderXRequestID); id != "" {
		return id
	}

	return c.Request().Header.Get(echo.HeaderXRequestID)
}

// User Entityから会員登録・Me用の安全なResponseを作成する。
func newUserResponse(user *entity.User) userResponse {
	return userResponse{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, Status: user.Status, CreatedAt: user.CreatedAt.UTC()}
}

// User EntityからLogin Response用のUser情報を作成する。
func newAuthUserResponse(user *entity.User) authUserResponse {
	return authUserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, Status: user.Status}
}
