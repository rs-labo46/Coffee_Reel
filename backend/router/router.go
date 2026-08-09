package router

import (
	"coffee-reel/controller"
	"coffee-reel/middleware"
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

type AdminComponents struct {
	Controller      controller.IAdminUserController
	Middleware      *middleware.AdminMiddleware
	VideoController controller.IAdminVideoController
}

type VideoComponents struct {
	Controller      controller.IVideoController
	SavedController controller.ISavedVideoController
	LikeController  controller.IVideoLikeController
}

func NewRouter(
	userController controller.IUserController,
	authMiddleware *middleware.AuthMiddleware,
	csrfMiddleware *middleware.CSRFMiddleware,
	rateLimitMiddleware *middleware.RateLimitMiddleware,
	frontendURL string,
	healthController controller.IHealthController,
	adminComponents AdminComponents,
	videoComponents ...VideoComponents,
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
		AllowOrigins: []string{frontendURL},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			echo.HeaderXCSRFToken,
			"Idempotency-Key",
		},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowCredentials: true,
	}))

	e.Use(middleware.BodyLimit())

	e.GET("/health", healthController.Check)

	e.POST("/signup", userController.SignUp, rateLimitMiddleware.Signup)
	e.POST("/login", userController.Login, rateLimitMiddleware.LoginIP)
	e.POST("/refresh", userController.Refresh, rateLimitMiddleware.Refresh, csrfMiddleware.Validate)
	e.POST("/logout", userController.Logout, csrfMiddleware.Validate)
	e.GET("/me", userController.Me, authMiddleware.Authenticate)

	adminGroup := e.Group("/admin", authMiddleware.Authenticate, adminComponents.Middleware.Authorize)
	adminGroup.GET("/users", adminComponents.Controller.ListUsers)
	adminGroup.GET("/users/:user_id", adminComponents.Controller.GetUserDetail)
	adminGroup.PATCH("/users/:user_id/suspend", adminComponents.Controller.SuspendUser)
	adminGroup.PATCH("/users/:user_id/resume", adminComponents.Controller.ResumeUser)

	if adminComponents.VideoController != nil {
		adminGroup.GET(
			"/videos",
			adminComponents.VideoController.List,
		)
		adminGroup.GET(
			"/videos/:video_id",
			adminComponents.VideoController.Detail,
		)
		adminGroup.PATCH(
			"/videos/:video_id/hide",
			adminComponents.VideoController.Hide,
		)
		adminGroup.PATCH(
			"/videos/:video_id/restore",
			adminComponents.VideoController.Restore,
		)
	}

	if len(videoComponents) == 1 {
		registerVideoRoutes(e, videoComponents[0], authMiddleware, rateLimitMiddleware)
	}

	return e
}

func registerVideoRoutes(
	e *echo.Echo,
	components VideoComponents,
	auth *middleware.AuthMiddleware,
	rateLimits *middleware.RateLimitMiddleware,
) {
	e.POST("/videos", components.Controller.StartUpload, auth.Authenticate, rateLimits.VideoStart)
	e.POST("/videos/:video_id/upload-complete", components.Controller.CompleteUpload, auth.Authenticate, rateLimits.VideoComplete)
	e.GET("/videos", components.Controller.ListReels, auth.OptionalAuthenticate)
	e.GET("/videos/:video_id", components.Controller.Detail, auth.OptionalAuthenticate)

	if components.LikeController != nil {
		e.PUT("/videos/:video_id/like", components.LikeController.Like, auth.Authenticate)
		e.DELETE("/videos/:video_id/like", components.LikeController.Unlike, auth.Authenticate)
	}

	e.GET("/me/videos", components.Controller.ListMine, auth.Authenticate)
	e.GET("/me/videos/:video_id", components.Controller.MineDetail, auth.Authenticate)
	e.PATCH("/me/videos/:video_id/private", components.Controller.SetPrivate, auth.Authenticate)
	e.PATCH("/me/videos/:video_id/publish", components.Controller.Republish, auth.Authenticate)
	e.DELETE("/me/videos/:video_id", components.Controller.Delete, auth.Authenticate)

	e.PUT("/videos/:video_id/saved", components.SavedController.Save, auth.Authenticate)
	e.DELETE("/videos/:video_id/saved", components.SavedController.Remove, auth.Authenticate)
	e.GET("/me/saved-videos", components.SavedController.List, auth.Authenticate)
}
