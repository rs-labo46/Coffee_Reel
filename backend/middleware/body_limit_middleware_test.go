package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestBodyLimitChunkedRequest(t *testing.T) {
	tests := []struct {
		name           string
		size           int
		wantStatus     int
		wantNextCalled bool
	}{
		{
			name:           "allows exactly 65536 bytes",
			size:           65536,
			wantStatus:     http.StatusNoContent,
			wantNextCalled: true,
		},
		{
			name:           "rejects 65537 bytes before next handler",
			size:           65537,
			wantStatus:     http.StatusRequestEntityTooLarge,
			wantNextCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			nextCalled := false

			e.Use(BodyLimit())
			e.POST("/", func(c echo.Context) error {
				nextCalled = true

				body, err := io.ReadAll(c.Request().Body)
				if err != nil {
					return err
				}
				if len(body) != tt.size {
					t.Fatalf("body size=%d, want %d", len(body), tt.size)
				}

				return c.NoContent(http.StatusNoContent)
			})

			req := httptest.NewRequest(
				http.MethodPost,
				"/",
				strings.NewReader(strings.Repeat("x", tt.size)),
			)
			req.ContentLength = -1
			req.TransferEncoding = []string{"chunked"}
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status=%d, want %d, body=%s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}
			if nextCalled != tt.wantNextCalled {
				t.Fatalf(
					"nextCalled=%t, want %t",
					nextCalled,
					tt.wantNextCalled,
				)
			}
		})
	}
}
