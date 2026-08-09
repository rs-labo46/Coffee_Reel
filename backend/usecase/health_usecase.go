package usecase

import (
	"context"
	"time"

	"coffee-reel/repository"
)

const healthCheckTimeout = 3 * time.Second

type IHealthUsecase interface {
	Check(ctx context.Context) error
}

type healthUsecase struct {
	health repository.IHealthRepository
}

func NewHealthUsecase(health repository.IHealthRepository) IHealthUsecase {
	return &healthUsecase{health: health}
}

func (u *healthUsecase) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	return u.health.Check(checkCtx)
}
