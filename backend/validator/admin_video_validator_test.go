package validator

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestAdminVideoValidatorValidateVideoID(t *testing.T) {
	validator := NewAdminVideoValidator()

	valid := []struct {
		raw  string
		want uint64
	}{
		{raw: "1", want: 1},
		{raw: fmt.Sprintf("%d", int64(math.MaxInt64)), want: uint64(math.MaxInt64)},
	}
	for _, tt := range valid {
		t.Run("valid "+tt.raw, func(t *testing.T) {
			got, err := validator.ValidateVideoID(tt.raw)
			if err != nil {
				t.Fatalf("ValidateVideoID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got = %d, want %d", got, tt.want)
			}
		})
	}

	invalid := []string{
		"",
		"0",
		"-1",
		"abc",
		"1.5",
		"9223372036854775808",
		"18446744073709551616",
	}
	for _, raw := range invalid {
		t.Run("invalid "+raw, func(t *testing.T) {
			got, err := validator.ValidateVideoID(raw)
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if got != 0 {
				t.Fatalf("got = %d, want 0", got)
			}
		})
	}
}

func TestAdminVideoValidatorValidateReason(t *testing.T) {
	validator := NewAdminVideoValidator()

	tests := []struct {
		name    string
		reason  string
		want    string
		wantErr bool
	}{
		{name: "one rune", reason: "あ", want: "あ"},
		{name: "trim", reason: "  規約違反  ", want: "規約違反"},
		{name: "500 runes", reason: strings.Repeat("あ", 500), want: strings.Repeat("あ", 500)},
		{name: "empty", reason: "", wantErr: true},
		{name: "spaces only", reason: "   ", wantErr: true},
		{name: "501 runes", reason: strings.Repeat("あ", 501), wantErr: true},
		{name: "invalid UTF-8", reason: string([]byte{0xff}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidateReason(tt.reason)
			if tt.wantErr {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateReason() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdminVideoValidatorValidateReasonRequest(t *testing.T) {
	validator := NewAdminVideoValidator()

	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "valid",
			body: `{"reason":"  規約違反  "}`,
			want: "規約違反",
		},
		{
			name:    "unknown field",
			body:    `{"reason":"規約違反","publish_status":"published"}`,
			wantErr: true,
		},
		{
			name:    "missing reason",
			body:    `{}`,
			wantErr: true,
		},
		{
			name:    "trailing JSON",
			body:    `{"reason":"規約違反"}{"reason":"別理由"}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			body:    `{"reason":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidateReasonRequest(strings.NewReader(tt.body))
			if tt.wantErr {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateReasonRequest() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got = %q, want %q", got, tt.want)
			}
		})
	}

	if _, err := validator.ValidateReasonRequest(nil); !errors.Is(err, entity.ErrInvalidInput) {
		t.Fatalf("nil body error = %v, want ErrInvalidInput", err)
	}
}

func TestAdminVideoValidatorValidateListQuery(t *testing.T) {
	validator := NewAdminVideoValidator()

	t.Run("default limit", func(t *testing.T) {
		input, err := validator.ValidateListQuery("", "")
		if err != nil {
			t.Fatalf("ValidateListQuery() error = %v", err)
		}
		if input.Limit != 20 || input.Cursor != nil {
			t.Fatalf("input = %#v", input)
		}
	})

	for _, raw := range []string{"1", "100"} {
		t.Run("valid limit "+raw, func(t *testing.T) {
			input, err := validator.ValidateListQuery(raw, "")
			if err != nil {
				t.Fatalf("ValidateListQuery() error = %v", err)
			}
			if fmt.Sprintf("%d", input.Limit) != raw {
				t.Fatalf("limit = %d, want %s", input.Limit, raw)
			}
		})
	}

	for _, raw := range []string{"0", "101", "abc", "-1"} {
		t.Run("invalid limit "+raw, func(t *testing.T) {
			_, err := validator.ValidateListQuery(raw, "")
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestAdminVideoValidatorValidateCursor(t *testing.T) {
	validator := NewAdminVideoValidator()
	createdAt := time.Date(2026, 8, 6, 1, 2, 3, 0, time.FixedZone("JST", 9*60*60))
	validJSON := fmt.Sprintf(
		`{"created_at":%q,"id":10}`,
		createdAt.Format(time.RFC3339Nano),
	)
	validCursor := base64.RawURLEncoding.EncodeToString([]byte(validJSON))

	input, err := validator.ValidateListQuery("20", validCursor)
	if err != nil {
		t.Fatalf("ValidateListQuery() error = %v", err)
	}
	if input.Cursor == nil ||
		input.Cursor.ID != 10 ||
		!input.Cursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("cursor = %#v", input.Cursor)
	}

	t.Run("JST cursor is accepted", func(t *testing.T) {
		offsetJSON := `{"created_at":"2026-08-06T10:02:03+09:00","id":10}`
		offsetCursor := base64.RawURLEncoding.EncodeToString([]byte(offsetJSON))

		input, err := validator.ValidateListQuery("20", offsetCursor)
		if err != nil {
			t.Fatalf("ValidateListQuery() error = %v", err)
		}
		want := time.Date(2026, 8, 6, 10, 2, 3, 0, time.FixedZone("JST", 9*60*60))
		if input.Cursor == nil ||
			!input.Cursor.CreatedAt.Equal(want) {
			t.Fatalf("cursor = %#v", input.Cursor)
		}
	})

	invalidJSONValues := []string{
		`{"created_at":"2026-08-06T10:02:03+09:00","id":0}`,
		`{"created_at":"2026-08-06T10:02:03+09:00","id":9223372036854775808}`,
		`{"created_at":"2026-08-06T10:02:03+09:00","id":10,"extra":true}`,
		`{"created_at":"2026-08-06T10:02:03+09:00","id":10}{}`,
	}

	invalidCursors := []string{"%%%"}
	for _, value := range invalidJSONValues {
		invalidCursors = append(
			invalidCursors,
			base64.RawURLEncoding.EncodeToString([]byte(value)),
		)
	}

	for index, cursor := range invalidCursors {
		t.Run(fmt.Sprintf("invalid cursor %d", index), func(t *testing.T) {
			_, err := validator.ValidateListQuery("20", cursor)
			if !errors.Is(err, entity.ErrCursorInvalid) {
				t.Fatalf("error = %v, want ErrCursorInvalid", err)
			}
		})
	}
}
