package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"coffee-reel/entity"

	"github.com/labstack/echo/v4"
)

func TestCSRFMiddlewareUsesFetchSiteAndDoubleSubmitCookie(t *testing.T) {
	tests := []struct {
		name        string
		fetchSite   string
		cookie      string
		header      string
		compareErr  error
		wantCompare bool
		wantNext    bool
		wantStatus  int
	}{
		{
			name:       "same origin is allowed by fetch metadata",
			fetchSite:  "same-origin",
			wantNext:   true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:        "cross site uses valid double submit cookie",
			fetchSite:   "cross-site",
			cookie:      "csrf-token",
			header:      "csrf-token",
			wantCompare: true,
			wantNext:    true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "same site still uses double submit cookie",
			fetchSite:   "same-site",
			cookie:      "csrf-token",
			header:      "csrf-token",
			wantCompare: true,
			wantNext:    true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "missing fetch site falls back to double submit cookie",
			cookie:      "csrf-token",
			header:      "csrf-token",
			wantCompare: true,
			wantNext:    true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "unknown fetch site falls back to double submit cookie",
			fetchSite:   "future-value",
			cookie:      "csrf-token",
			header:      "csrf-token",
			wantCompare: true,
			wantNext:    true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:       "missing cookie is rejected",
			fetchSite:  "cross-site",
			header:     "csrf-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:        "missing header is rejected",
			fetchSite:   "cross-site",
			cookie:      "csrf-token",
			compareErr:  entity.ErrCSRFInvalid,
			wantCompare: true,
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "mismatch is rejected",
			fetchSite:   "cross-site",
			cookie:      "csrf-token",
			header:      "other",
			compareErr:  entity.ErrCSRFInvalid,
			wantCompare: true,
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "comparison failure is fail closed",
			fetchSite:   "cross-site",
			cookie:      "csrf-token",
			header:      "csrf-token",
			compareErr:  errors.New("comparison failed"),
			wantCompare: true,
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compareCalled := false

			tokens := &tokenServiceMock{
				compareCSRFTokenFunc: func(
					cookie string,
					header string,
				) error {
					compareCalled = true

					if cookie != tt.cookie || header != tt.header {
						t.Fatalf(
							"CompareCSRFToken(%q, %q)",
							cookie,
							header,
						)
					}

					return tt.compareErr
				},
			}

			e := echo.New()

			req := httptest.NewRequest(
				http.MethodPost,
				"/refresh",
				nil,
			)
			req.Header.Set(
				echo.HeaderXRequestID,
				"request-csrf",
			)

			if tt.fetchSite != "" {
				req.Header.Set(
					fetchSiteHeaderName,
					tt.fetchSite,
				)
			}

			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{
					Name:  csrfCookieName,
					Value: tt.cookie,
				})
			}

			if tt.header != "" {
				req.Header.Set(
					csrfHeaderName,
					tt.header,
				)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			nextCalled := false

			err := NewCSRFMiddleware(tokens).Validate(
				func(c echo.Context) error {
					nextCalled = true

					return c.NoContent(
						http.StatusNoContent,
					)
				},
			)(c)

			if err != nil {
				t.Fatalf(
					"Validate() returned Echo error = %v",
					err,
				)
			}

			if compareCalled != tt.wantCompare {
				t.Fatalf(
					"compareCalled = %v, want %v",
					compareCalled,
					tt.wantCompare,
				)
			}

			if nextCalled != tt.wantNext {
				t.Fatalf(
					"nextCalled = %v, want %v",
					nextCalled,
					tt.wantNext,
				)
			}

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d, body=%s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}

			if !tt.wantNext {
				if !strings.Contains(
					rec.Body.String(),
					`"code":"csrf_invalid"`,
				) ||
					!strings.Contains(
						rec.Body.String(),
						`"request_id":"request-csrf"`,
					) {
					t.Fatalf(
						"invalid CSRF error body = %s",
						rec.Body.String(),
					)
				}

				if tt.cookie != "" &&
					strings.Contains(
						rec.Body.String(),
						tt.cookie,
					) {
					t.Fatalf(
						"CSRF cookie leaked in response: %s",
						rec.Body.String(),
					)
				}

				if tt.header != "" &&
					strings.Contains(
						rec.Body.String(),
						tt.header,
					) {
					t.Fatalf(
						"CSRF header leaked in response: %s",
						rec.Body.String(),
					)
				}
			}
		})
	}
}
