package validator

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
	"coffee-reel/usecase"
)

func newVideoValidatorForTest(t *testing.T, maxKeyBytes int) IVideoValidator {
	t.Helper()

	validator, err := NewVideoValidator(VideoValidatorConfig{
		IdempotencyKeyMaxBytes: maxKeyBytes,
	})
	if err != nil {
		t.Fatalf("NewVideoValidator() error = %v", err)
	}
	return validator
}

func TestNewVideoValidatorRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		maxBytes int
	}{
		{name: "zero", maxBytes: 0},
		{name: "negative", maxBytes: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator, err := NewVideoValidator(VideoValidatorConfig{
				IdempotencyKeyMaxBytes: tt.maxBytes,
			})
			if err == nil {
				t.Fatal("NewVideoValidator() error = nil, want error")
			}
			if validator != nil {
				t.Fatalf("validator = %#v, want nil", validator)
			}
		})
	}

	validator, err := NewVideoValidator(VideoValidatorConfig{
		IdempotencyKeyMaxBytes: 1,
	})
	if err != nil {
		t.Fatalf("NewVideoValidator(valid) error = %v", err)
	}
	if validator == nil {
		t.Fatal("NewVideoValidator(valid) returned nil")
	}
}

func TestVideoValidatorValidateStartUploadNormalizesOnlySpecifiedFields(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)
	description := "  抽出方法の説明  "

	input, err := validator.ValidateStartUpload(
		"  ハンドドリップ  ",
		description,
		string(entity.CategoryBrewing),
		" VIDEO/MP4 ",
		1_024,
	)
	if err != nil {
		t.Fatalf("ValidateStartUpload() error = %v", err)
	}
	if input.Title != "ハンドドリップ" {
		t.Fatalf("Title = %q, want %q", input.Title, "ハンドドリップ")
	}
	if input.Description != description {
		t.Fatalf("Description = %q, want unchanged %q", input.Description, description)
	}
	if input.Category != entity.CategoryBrewing {
		t.Fatalf("Category = %q, want %q", input.Category, entity.CategoryBrewing)
	}
	if input.ContentType != "video/mp4" {
		t.Fatalf("ContentType = %q, want video/mp4", input.ContentType)
	}
	if input.DeclaredSize != 1_024 {
		t.Fatalf("DeclaredSize = %d, want 1024", input.DeclaredSize)
	}
}

func TestVideoValidatorValidateStartUploadTitleBoundaries(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	tests := []struct {
		name      string
		title     string
		wantTitle string
		wantError bool
	}{
		{name: "empty", title: "", wantError: true},
		{name: "spaces only", title: " \t\n ", wantError: true},
		{name: "one rune", title: "珈", wantTitle: "珈"},
		{name: "one hundred runes", title: strings.Repeat("珈", 100), wantTitle: strings.Repeat("珈", 100)},
		{name: "one hundred one runes", title: strings.Repeat("珈", 101), wantError: true},
		{name: "trim before counting", title: "  " + strings.Repeat("珈", 100) + "  ", wantTitle: strings.Repeat("珈", 100)},
		{name: "invalid utf8", title: string([]byte{0xff}), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := validator.ValidateStartUpload(
				tt.title,
				"",
				string(entity.CategoryBrewing),
				"video/mp4",
				1,
			)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if input.Title != tt.wantTitle {
				t.Fatalf("Title = %q, want %q", input.Title, tt.wantTitle)
			}
		})
	}
}

func TestVideoValidatorValidateStartUploadDescriptionBoundaries(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	tests := []struct {
		name        string
		description string
		wantError   bool
	}{
		{name: "empty", description: ""},
		{name: "one thousand runes", description: strings.Repeat("珈", 1_000)},
		{name: "one thousand one runes", description: strings.Repeat("珈", 1_001), wantError: true},
		{name: "invalid utf8", description: string([]byte{0xff}), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := validator.ValidateStartUpload(
				"title",
				tt.description,
				string(entity.CategoryBrewing),
				"video/mp4",
				1,
			)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if input.Description != tt.description {
				t.Fatalf("Description was changed: got %q, want %q", input.Description, tt.description)
			}
		})
	}
}

func TestVideoValidatorValidateStartUploadCategories(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	validCategories := []entity.CategoryCode{
		entity.CategoryBrewing,
		entity.CategoryRoasting,
		entity.CategoryLatteArt,
		entity.CategoryBeans,
		entity.CategoryEquipment,
	}
	for _, category := range validCategories {
		t.Run(string(category), func(t *testing.T) {
			input, err := validator.ValidateStartUpload(
				"title",
				"",
				string(category),
				"video/mp4",
				1,
			)
			if err != nil {
				t.Fatalf("ValidateStartUpload() error = %v", err)
			}
			if input.Category != category {
				t.Fatalf("Category = %q, want %q", input.Category, category)
			}
		})
	}

	for _, category := range []string{
		"",
		"Brewing",
		" brewing ",
		"unknown",
		string([]byte{0xff}),
	} {
		t.Run("invalid_"+base64.RawURLEncoding.EncodeToString([]byte(category)), func(t *testing.T) {
			_, err := validator.ValidateStartUpload(
				"title",
				"",
				category,
				"video/mp4",
				1,
			)
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("category %q error = %v, want ErrInvalidInput", category, err)
			}
		})
	}
}

func TestVideoValidatorValidateStartUploadContentTypes(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	tests := []struct {
		name        string
		contentType string
		want        string
		wantError   bool
	}{
		{name: "mp4", contentType: "video/mp4", want: "video/mp4"},
		{name: "mp4 normalized", contentType: " VIDEO/MP4 ", want: "video/mp4"},
		{name: "quicktime", contentType: "video/quicktime", want: "video/quicktime"},
		{name: "quicktime normalized", contentType: " Video/QuickTime ", want: "video/quicktime"},
		{name: "empty", contentType: "", wantError: true},
		{name: "unsupported", contentType: "video/webm", wantError: true},
		{name: "parameters rejected", contentType: "video/mp4; charset=binary", wantError: true},
		{name: "invalid utf8", contentType: string([]byte{0xff}), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := validator.ValidateStartUpload(
				"title",
				"",
				string(entity.CategoryBrewing),
				tt.contentType,
				1,
			)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if input.ContentType != tt.want {
				t.Fatalf("ContentType = %q, want %q", input.ContentType, tt.want)
			}
		})
	}
}

func TestVideoValidatorValidateStartUploadDeclaredSizeBoundaries(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	tests := []struct {
		name      string
		size      int64
		wantError bool
	}{
		{name: "negative", size: -1, wantError: true},
		{name: "zero", size: 0, wantError: true},
		{name: "minimum", size: 1},
		{name: "maximum", size: 30_000_000},
		{name: "over maximum", size: 30_000_001, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := validator.ValidateStartUpload(
				"title",
				"",
				string(entity.CategoryBrewing),
				"video/mp4",
				tt.size,
			)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if input.DeclaredSize != tt.size {
				t.Fatalf("DeclaredSize = %d, want %d", input.DeclaredSize, tt.size)
			}
		})
	}
}

func TestVideoValidatorValidateIdempotencyKeyUsesUTF8BytesAndRejectsWhitespace(t *testing.T) {
	validator := newVideoValidatorForTest(t, 6)

	tests := []struct {
		name      string
		key       string
		wantError bool
	}{
		{name: "one byte", key: "a"},
		{name: "exact ascii byte limit", key: "abcdef"},
		{name: "exact multibyte byte limit", key: "珈珈"},
		{name: "over ascii byte limit", key: "abcdefg", wantError: true},
		{name: "over multibyte byte limit", key: "珈珈a", wantError: true},
		{name: "empty", key: "", wantError: true},
		{name: "space", key: "abc def", wantError: true},
		{name: "tab", key: "abc\tdef", wantError: true},
		{name: "newline", key: "abc\ndef", wantError: true},
		{name: "full width space", key: "abc　def", wantError: true},
		{name: "invalid utf8", key: string([]byte{0xff}), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidateIdempotencyKey(tt.key)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				if got != "" {
					t.Fatalf("key = %q, want empty", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if got != tt.key {
				t.Fatalf("key was changed: got %q, want %q", got, tt.key)
			}
		})
	}
}

func TestVideoValidatorValidateVideoID(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	valid := []struct {
		raw  string
		want uint64
	}{
		{raw: "1", want: 1},
		{raw: "9223372036854775807", want: math.MaxInt64},
	}
	for _, tt := range valid {
		t.Run("valid_"+tt.raw, func(t *testing.T) {
			got, err := validator.ValidateVideoID(tt.raw)
			if err != nil {
				t.Fatalf("ValidateVideoID(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ValidateVideoID(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}

	for _, raw := range []string{
		"",
		"0",
		"-1",
		"+1",
		"1.0",
		"abc",
		" 1",
		"1 ",
		"9223372036854775808",
		"18446744073709551615",
		"18446744073709551616",
	} {
		t.Run("invalid_"+base64.RawURLEncoding.EncodeToString([]byte(raw)), func(t *testing.T) {
			got, err := validator.ValidateVideoID(raw)
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("ValidateVideoID(%q) error = %v, want ErrInvalidInput", raw, err)
			}
			if got != 0 {
				t.Fatalf("ValidateVideoID(%q) = %d, want 0", raw, got)
			}
		})
	}
}

func TestVideoValidatorValidateListQueryDefaultsAndLimitBoundaries(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	input, err := validator.ValidateListQuery("", "")
	if err != nil {
		t.Fatalf("ValidateListQuery(default) error = %v", err)
	}
	if input.Limit != 20 || input.Cursor != nil {
		t.Fatalf("default input = %#v, want Limit 20 and nil Cursor", input)
	}

	for _, tt := range []struct {
		raw  string
		want int
	}{
		{raw: "1", want: 1},
		{raw: "20", want: 20},
		{raw: "100", want: 100},
	} {
		t.Run("valid_"+tt.raw, func(t *testing.T) {
			got, err := validator.ValidateListQuery(tt.raw, "")
			if err != nil {
				t.Fatalf("ValidateListQuery(%q) error = %v", tt.raw, err)
			}
			if got.Limit != tt.want || got.Cursor != nil {
				t.Fatalf("input = %#v, want Limit %d and nil Cursor", got, tt.want)
			}
		})
	}

	for _, raw := range []string{"0", "-1", "101", "abc", "1.5", " 20", "20 "} {
		t.Run("invalid_"+base64.RawURLEncoding.EncodeToString([]byte(raw)), func(t *testing.T) {
			input, err := validator.ValidateListQuery(raw, "")
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("limit %q error = %v, want ErrInvalidInput", raw, err)
			}
			if input != (usecase.VideoListInput{}) {
				t.Fatalf("input = %#v, want zero value", input)
			}
		})
	}
}

func TestVideoValidatorValidateListQueryDecodesValidCursor(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	cursor := usecase.VideoCursor{
		CreatedAt: time.Date(2026, 8, 4, 0, 0, 0, 123, time.UTC),
		ID:        math.MaxInt64,
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)

	input, err := validator.ValidateListQuery("100", encoded)
	if err != nil {
		t.Fatalf("ValidateListQuery() error = %v", err)
	}
	if input.Limit != 100 {
		t.Fatalf("Limit = %d, want 100", input.Limit)
	}
	if input.Cursor == nil {
		t.Fatal("Cursor = nil")
	}
	if input.Cursor.ID != math.MaxInt64 {
		t.Fatalf("Cursor.ID = %d, want %d", input.Cursor.ID, uint64(math.MaxInt64))
	}
	if !input.Cursor.CreatedAt.Equal(cursor.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", input.Cursor.CreatedAt, cursor.CreatedAt)
	}
	if input.Cursor.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt.Location = %v, want UTC", input.Cursor.CreatedAt.Location())
	}
}

func TestVideoValidatorValidateListQueryNormalizesZeroOffsetCursorToUTC(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(
		`{"created_at":"2026-08-04T00:00:00+00:00","id":1}`,
	))

	input, err := validator.ValidateListQuery("20", encoded)
	if err != nil {
		t.Fatalf("ValidateListQuery() error = %v", err)
	}
	if input.Cursor == nil {
		t.Fatal("Cursor = nil")
	}
	if input.Cursor.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt.Location = %v, want UTC", input.Cursor.CreatedAt.Location())
	}
}

func TestVideoValidatorRejectsInvalidCursor(t *testing.T) {
	validator := newVideoValidatorForTest(t, 128)

	tests := []struct {
		name   string
		cursor string
	}{
		{name: "invalid base64", cursor: "not-base64"},
		{name: "padded base64", cursor: base64.URLEncoding.EncodeToString([]byte(`{"created_at":"2026-08-04T00:00:00Z","id":1}`))},
		{name: "not json", cursor: encodeRawCursor(`not-json`)},
		{name: "unknown field", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z","id":1,"extra":true}`)},
		{name: "missing created at", cursor: encodeRawCursor(`{"id":1}`)},
		{name: "missing id", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z"}`)},
		{name: "zero created at", cursor: encodeRawCursor(`{"created_at":"0001-01-01T00:00:00Z","id":1}`)},
		{name: "non utc", cursor: encodeRawCursor(`{"created_at":"2026-08-04T09:00:00+09:00","id":1}`)},
		{name: "zero id", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z","id":0}`)},
		{name: "id above max int64", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z","id":9223372036854775808}`)},
		{name: "wrong id type", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z","id":"1"}`)},
		{name: "wrong date type", cursor: encodeRawCursor(`{"created_at":1,"id":1}`)},
		{name: "second json object", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z","id":1}{"created_at":"2026-08-03T00:00:00Z","id":2}`)},
		{name: "trailing null", cursor: encodeRawCursor(`{"created_at":"2026-08-04T00:00:00Z","id":1} null`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := validator.ValidateListQuery("20", tt.cursor)
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if input != (usecase.VideoListInput{}) {
				t.Fatalf("input = %#v, want zero value", input)
			}
		})
	}
}

func encodeRawCursor(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
