package router

import (
	"coffee-reel/controller"
	"coffee-reel/middleware"
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

// 共通Middlewareと認証APIをEchoへ接続する。
func NewRouter(
	userController controller.IUserController,
	authMiddleware *middleware.AuthMiddleware,
	csrfMiddleware *middleware.CSRFMiddleware,
	rateLimitMiddleware *middleware.RateLimitMiddleware,
	frontendURL string,
) *echo.Echo {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(echomw.Recover())
	e.Use(echomw.RequestID())
	e.Use(echomw.Logger())

	e.Use(echomw.SecureWithConfig(echomw.SecureConfig{
		XSSProtection:         "0",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
		ReferrerPolicy:        "no-referrer",
	}))

	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: []string{
			frontendURL,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			echo.HeaderXCSRFToken,
		},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodOptions,
		},
		AllowCredentials: true,
	}))

	e.Use(echomw.BodyLimit("65536B"))
	e.POST("/signup", userController.SignUp, rateLimitMiddleware.Signup)
	e.POST("/login", userController.Login, rateLimitMiddleware.LoginIP)
	e.POST("/refresh", userController.Refresh, rateLimitMiddleware.Refresh, csrfMiddleware.Validate)
	e.POST("/logout", userController.Logout, csrfMiddleware.Validate)
	e.GET("/me", userController.Me, authMiddleware.Authenticate)

	return e
}
