package validator

import (
	"coffee-reel/entity"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLength     = 50
	maxEmailLength    = 254
	minPasswordLength = 8
	maxPasswordBytes  = 72
)

type IUserValidator interface {
	ValidateSignup(name, email, password string) (string, string, string, error)
	ValidateLogin(email, password string) (string, string, error)
}

type userValidator struct{}

func NewUserValidator() IUserValidator {
	return &userValidator{}
}

// 会員登録入力を検証し、正規化済みのNameとEmailを返す。
func (v *userValidator) ValidateSignup(name, email, password string) (string, string, string, error) {
	name = strings.TrimSpace(name)

	if !utf8.ValidString(name) {
		return "", "", "", entity.ErrInvalidInput
	}

	nameLength := utf8.RuneCountInString(name)
	if nameLength < 1 || nameLength > maxNameLength {
		return "", "", "", entity.ErrInvalidInput
	}

	normalizedEmail, err := normalizeAndValidateEmail(email)
	if err != nil {
		return "", "", "", err
	}

	if !utf8.ValidString(password) {
		return "", "", "", entity.ErrInvalidInput
	}

	passwordLength := utf8.RuneCountInString(password)
	if passwordLength < minPasswordLength || len(password) > maxPasswordBytes {
		return "", "", "", entity.ErrInvalidInput
	}

	return name, normalizedEmail, password, nil
}

// Login入力を検証し、正規化済みEmailと未変更のPasswordを返す。
func (v *userValidator) ValidateLogin(email, password string) (string, string, error) {
	normalizedEmail, err := normalizeAndValidateEmail(email)
	if err != nil {
		return "", "", err
	}

	if password == "" || !utf8.ValidString(password) {
		return "", "", entity.ErrInvalidInput
	}

	return normalizedEmail, password, nil
}

// Emailをtrim・小文字化し、長さと形式を検証する。
func normalizeAndValidateEmail(email string) (string, error) {

	if !utf8.ValidString(email) {
		return "", entity.ErrInvalidInput
	}

	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		return "", entity.ErrInvalidInput
	}

	if utf8.RuneCountInString(email) > maxEmailLength {
		return "", entity.ErrInvalidInput
	}

	address, err := mail.ParseAddress(email)
	if err != nil {
		return "", entity.ErrInvalidInput
	}

	// 表示名付きの形式ではなく、Email Address単体だけを許可する。
	if address.Name != "" || address.Address != email {
		return "", entity.ErrInvalidInput
	}

	return email, nil
}
