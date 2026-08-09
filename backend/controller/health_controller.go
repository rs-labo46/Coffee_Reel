package controller

import (
	"net/http"

	"coffee-reel/usecase"

	"github.com/labstack/echo/v4"
)

type IHealthController interface {
	Check(c echo.Context) error
}

type healthController struct {
	health usecase.IHealthUsecase
}

type healthResponse struct {
	Status string `json:"status"`
}

func NewHealthController(health usecase.IHealthUsecase) IHealthController {
	return &healthController{health: health}
}

func (h *healthController) Check(c echo.Context) error {
	if err := h.health.Check(c.Request().Context()); err != nil {
		return c.JSON(http.StatusServiceUnavailable, healthResponse{Status: "unavailable"})
	}

	return c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}
