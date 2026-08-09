package validator

import (
	"bytes"
	"coffee-reel/entity"
	"coffee-reel/usecase"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	defaultAdminUserListLimit = 20
	maxAdminUserListLimit     = 100
	maxAdminReasonLength      = 500
)

type IAdminUserValidator interface {
	ValidateCreateAdmin(name, email, password string) (string, string, string, error)
	ValidateUserID(rawUserID string) (uint64, error)
	ValidateReason(reason string) (string, error)
	ValidateUserListQuery(rawLimit, rawCursor string) (usecase.AdminUserListInput, error)
}

type adminUserValidator struct {
	users IUserValidator
}

func NewAdminUserValidator(users IUserValidator) IAdminUserValidator {
	return &adminUserValidator{users: users}
}

func (v *adminUserValidator) ValidateCreateAdmin(name, email, password string) (string, string, string, error) {
	return v.users.ValidateSignup(name, email, password)
}

func (v *adminUserValidator) ValidateUserID(rawUserID string) (uint64, error) {
	if rawUserID == "" || strings.HasPrefix(rawUserID, "-") {
		return 0, entity.ErrInvalidInput
	}

	userID, err := strconv.ParseUint(rawUserID, 10, 64)
	if err != nil || userID == 0 || userID > math.MaxInt64 {
		return 0, entity.ErrInvalidInput
	}

	return userID, nil
}

func (v *adminUserValidator) ValidateReason(reason string) (string, error) {
	if !utf8.ValidString(reason) {
		return "", entity.ErrInvalidInput
	}

	reason = strings.TrimSpace(reason)
	length := utf8.RuneCountInString(reason)
	if length < 1 || length > maxAdminReasonLength {
		return "", entity.ErrInvalidInput
	}

	return reason, nil
}

func (v *adminUserValidator) ValidateUserListQuery(rawLimit, rawCursor string) (usecase.AdminUserListInput, error) {
	limit := defaultAdminUserListLimit
	if rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > maxAdminUserListLimit {
			return usecase.AdminUserListInput{}, entity.ErrInvalidInput
		}
		limit = parsedLimit
	}

	input := usecase.AdminUserListInput{Limit: limit}
	if rawCursor == "" {
		return input, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil {
		return usecase.AdminUserListInput{}, entity.ErrInvalidInput
	}

	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()

	var cursor usecase.AdminUserCursor
	if err := decoder.Decode(&cursor); err != nil {
		return usecase.AdminUserListInput{}, entity.ErrInvalidInput
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return usecase.AdminUserListInput{}, entity.ErrInvalidInput
	}

	if cursor.CreatedAt.IsZero() ||
		cursor.ID == 0 ||
		cursor.ID > math.MaxInt64 {
		return usecase.AdminUserListInput{}, entity.ErrInvalidInput
	}

	input.Cursor = &cursor

	return input, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return entity.ErrInvalidInput
	}
	return nil
}
