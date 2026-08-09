package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

type healthRepositoryMock struct {
	checkFunc func(context.Context) error
}

func (m *healthRepositoryMock) Check(ctx context.Context) error {
	if m.checkFunc == nil {
		panic("unexpected HealthRepository.Check call")
	}
	return m.checkFunc(ctx)
}

func TestHealthUsecaseCheck(t *testing.T) {
	repo := &healthRepositoryMock{
		checkFunc: func(context.Context) error {
			return nil
		},
	}

	uc := NewHealthUsecase(repo)
	if err := uc.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestHealthUsecaseCheckReturnsRepositoryError(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	repo := &healthRepositoryMock{
		checkFunc: func(context.Context) error {
			return wantErr
		},
	}

	uc := NewHealthUsecase(repo)
	err := uc.Check(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Check() error = %v, want %v", err, wantErr)
	}
}

func TestHealthUsecaseCheckUsesTimeout(t *testing.T) {
	repo := &healthRepositoryMock{
		checkFunc: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("health check context has no deadline")
			}

			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > healthCheckTimeout {
				t.Fatalf("remaining timeout = %s, want > 0 and <= %s", remaining, healthCheckTimeout)
			}
			return nil
		},
	}

	uc := NewHealthUsecase(repo)
	if err := uc.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}
