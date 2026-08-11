package controller

import (
	"time"

	"coffee-reel/usecase"
)

func (m *userUsecaseMock) IssueCSRFToken() (usecase.CSRFTokenResult, error) {
	return usecase.CSRFTokenResult{
		Token:     "bootstrap-csrf",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second),
	}, nil
}
