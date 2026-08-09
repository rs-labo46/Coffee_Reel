package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

const jsonRequestBodyLimitBytes int64 = 65536

func BodyLimit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.Body == nil {
				return next(c)
			}

			if req.ContentLength > jsonRequestBodyLimitBytes {
				return writeMiddlewareError(
					c,
					http.StatusRequestEntityTooLarge,
					"request_too_large",
					"リクエストサイズが上限を超えています",
				)
			}

			// Content-LengthがなくてもController前に上限超過を判定するため、上限+1 byteだけ先読みする。
			originalBody := req.Body
			body, err := io.ReadAll(io.LimitReader(originalBody, jsonRequestBodyLimitBytes+1))
			if err != nil {
				return err
			}
			if int64(len(body)) > jsonRequestBodyLimitBytes {
				return writeMiddlewareError(
					c,
					http.StatusRequestEntityTooLarge,
					"request_too_large",
					"リクエストサイズが上限を超えています",
				)
			}

			req.Body = struct {
				io.Reader
				io.Closer
			}{
				Reader: bytes.NewReader(body),
				Closer: originalBody,
			}

			return next(c)
		}
	}
}
