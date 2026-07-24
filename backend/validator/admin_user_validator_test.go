package validator

import (
	"coffee-reel/entity"
	"coffee-reel/usecase"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func newAdminUserValidatorForTest() IAdminUserValidator {
	return NewAdminUserValidator(NewUserValidator())
}

func TestAdminUserValidatorValidateCreateAdmin(t *testing.T) {
	validator := newAdminUserValidatorForTest()
	name, email, password, err := validator.ValidateCreateAdmin(
		"  管理者  ",
		" ADMIN@EXAMPLE.COM ",
		"password123",
	)
	if err != nil {
		t.Fatalf("ValidateCreateAdmin() error = %v", err)
	}
	if name != "管理者" || email != "admin@example.com" || password != "password123" {
		t.Fatalf("normalized values = %q %q %q", name, email, password)
	}
}

func TestAdminUserValidatorValidateUserID(t *testing.T) {
	validator := newAdminUserValidatorForTest()

	for _, value := range []string{"", "0", "-1", "abc", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			if _, err := validator.ValidateUserID(value); !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("ValidateUserID(%q) error = %v", value, err)
			}
		})
	}

	id, err := validator.ValidateUserID("9223372036854775807")
	if err != nil || id != math.MaxInt64 {
		t.Fatalf("ValidateUserID(max) = %d, %v", id, err)
	}
}

func TestAdminUserValidatorValidateReason(t *testing.T) {
	validator := newAdminUserValidatorForTest()

	reason, err := validator.ValidateReason("  確認完了  ")
	if err != nil || reason != "確認完了" {
		t.Fatalf("ValidateReason() = %q, %v", reason, err)
	}

	for _, value := range []string{"", "   ", strings.Repeat("あ", 501)} {
		if _, err := validator.ValidateReason(value); !errors.Is(err, entity.ErrInvalidInput) {
			t.Fatalf("ValidateReason(%q) error = %v", value, err)
		}
	}
}

func TestAdminUserValidatorValidateUserListQuery(t *testing.T) {
	validator := newAdminUserValidatorForTest()

	input, err := validator.ValidateUserListQuery("", "")
	if err != nil || input.Limit != 20 || input.Cursor != nil {
		t.Fatalf("default query = %#v, %v", input, err)
	}

	for _, value := range []string{"0", "-1", "101", "abc"} {
		if _, err := validator.ValidateUserListQuery(value, ""); !errors.Is(err, entity.ErrInvalidInput) {
			t.Fatalf("limit %q error = %v", value, err)
		}
	}

	cursor := usecase.AdminUserCursor{
		CreatedAt: time.Date(2026, 7, 23, 10, 0, 0, 123, time.UTC),
		ID:        25,
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)

	input, err = validator.ValidateUserListQuery("100", encoded)
	if err != nil {
		t.Fatalf("valid cursor error = %v", err)
	}
	if input.Limit != 100 || input.Cursor == nil || input.Cursor.ID != 25 || !input.Cursor.CreatedAt.Equal(cursor.CreatedAt) {
		t.Fatalf("decoded input = %#v", input)
	}
}

func TestAdminUserValidatorRejectsInvalidCursor(t *testing.T) {
	validator := newAdminUserValidatorForTest()

	unknownField := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-07-23T10:00:00Z","id":1,"extra":true}`))
	nonUTC := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-07-23T19:00:00+09:00","id":1}`))
	zeroID := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-07-23T10:00:00Z","id":0}`))
	tooLargeID := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-07-23T10:00:00Z","id":9223372036854775808}`))

	for _, value := range []string{
		"not-base64",
		base64.RawURLEncoding.EncodeToString([]byte(`not-json`)),
		unknownField,
		nonUTC,
		zeroID,
		tooLargeID,
	} {
		if _, err := validator.ValidateUserListQuery("20", value); !errors.Is(err, entity.ErrInvalidInput) {
			t.Fatalf("cursor %q error = %v", value, err)
		}
	}
}
