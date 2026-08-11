package usecase

import (
	"errors"
	"testing"
	"time"
)

func TestUserUsecaseIssueCSRFToken(t *testing.T) {
	tokens := &tokenServiceMock{
		generateCSRFTokenFunc: func() (string, error) {
			return "csrf-token", nil
		},
	}
	uc := NewUserUsecase(&userRepositoryMock{}, &refreshTokenRepositoryMock{}, tokens)

	before := time.Now().Add(refreshTokenLifetime)
	result, err := uc.IssueCSRFToken()
	after := time.Now().Add(refreshTokenLifetime)

	if err != nil {
		t.Fatalf("IssueCSRFToken() error = %v", err)
	}
	if result.Token != "csrf-token" {
		t.Fatalf("token = %q", result.Token)
	}
	if result.ExpiresAt.Before(before) || result.ExpiresAt.After(after) {
		t.Fatalf("expires_at = %s, want between %s and %s", result.ExpiresAt, before, after)
	}
}

func TestUserUsecaseIssueCSRFTokenReturnsGenerationError(t *testing.T) {
	wantErr := errors.New("generate csrf failed")
	tokens := &tokenServiceMock{
		generateCSRFTokenFunc: func() (string, error) {
			return "", wantErr
		},
	}
	uc := NewUserUsecase(&userRepositoryMock{}, &refreshTokenRepositoryMock{}, tokens)

	result, err := uc.IssueCSRFToken()
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result.Token != "" || !result.ExpiresAt.IsZero() {
		t.Fatalf("result = %+v", result)
	}
}
