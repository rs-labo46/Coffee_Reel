package validator

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"coffee-reel/entity"
)

func TestValidateSignupNormalizesNameAndEmailWithoutChangingPassword(t *testing.T) {
	validator := NewUserValidator()
	password := "  password  "

	name, email, gotPassword, err := validator.ValidateSignup("  山田 太郎  ", "  USER@Example.COM  ", password)
	if err != nil {
		t.Fatalf("ValidateSignup() error = %v", err)
	}
	if name != "山田 太郎" {
		t.Fatalf("name = %q, want %q", name, "山田 太郎")
	}
	if email != "user@example.com" {
		t.Fatalf("email = %q, want user@example.com", email)
	}
	if gotPassword != password {
		t.Fatalf("password was changed: got %q, want %q", gotPassword, password)
	}
}

func TestValidateSignupNameBoundaries(t *testing.T) {
	validator := NewUserValidator()
	validEmail := "user@example.com"
	validPassword := "password"

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{name: "empty", input: "", wantError: true},
		{name: "spaces only", input: "   ", wantError: true},
		{name: "one rune", input: "珈", wantError: false},
		{name: "fifty runes", input: strings.Repeat("珈", 50), wantError: false},
		{name: "fifty one runes", input: strings.Repeat("珈", 51), wantError: true},
		{name: "trim before counting", input: "  " + strings.Repeat("珈", 50) + "  ", wantError: false},
		{name: "invalid utf8", input: string([]byte{0xff}), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := validator.ValidateSignup(tt.input, validEmail, validPassword)
			if tt.wantError && !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
		})
	}
}

func TestValidateSignupPasswordBoundariesUseRunesAndUTF8Bytes(t *testing.T) {
	validator := NewUserValidator()

	tests := []struct {
		name      string
		password  string
		wantError bool
	}{
		{name: "seven ascii characters", password: strings.Repeat("a", 7), wantError: true},
		{name: "eight ascii characters", password: strings.Repeat("a", 8), wantError: false},
		{name: "seventy two ascii bytes", password: strings.Repeat("a", 72), wantError: false},
		{name: "seventy three ascii bytes", password: strings.Repeat("a", 73), wantError: true},
		{name: "eight multibyte runes within limit", password: strings.Repeat("珈", 8), wantError: false},
		{name: "twenty four multibyte runes exactly seventy two bytes", password: strings.Repeat("珈", 24), wantError: false},
		{name: "twenty five multibyte runes exceed seventy two bytes", password: strings.Repeat("珈", 25), wantError: true},
		{name: "invalid utf8", password: string([]byte{0xff, 0xfe, 0xfd, 'a', 'b', 'c', 'd', 'e'}), wantError: true},
		{name: "leading and trailing spaces count and are preserved", password: "  pass  ", wantError: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, password, err := validator.ValidateSignup("user", "user@example.com", tt.password)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if password != tt.password {
				t.Fatalf("password was normalized: got %q, want %q", password, tt.password)
			}
		})
	}
}

func TestValidateEmailNormalizationAndFormat(t *testing.T) {
	validator := NewUserValidator()
	localPart := strings.Repeat("a", 64)
	domainLabel := strings.Repeat("b", 63)
	valid254 := localPart + "@" + domainLabel + "." + domainLabel + "." + strings.Repeat("c", 61)
	if utf8.RuneCountInString(valid254) != 254 {
		t.Fatalf("test setup: valid254 length = %d", utf8.RuneCountInString(valid254))
	}

	tests := []struct {
		name      string
		email     string
		want      string
		wantError bool
	}{
		{name: "trim and lowercase", email: "  User.Name+tag@Example.COM  ", want: "user.name+tag@example.com"},
		{name: "maximum length", email: valid254, want: valid254},
		{name: "empty", email: "", wantError: true},
		{name: "spaces only", email: "   ", wantError: true},
		{name: "missing at", email: "example.com", wantError: true},
		{name: "display name is rejected", email: "Alice <alice@example.com>", wantError: true},
		{name: "multiple addresses are rejected", email: "a@example.com,b@example.com", wantError: true},
		{name: "longer than 254", email: valid254 + "x", wantError: true},
		{name: "invalid utf8", email: string([]byte{'a', '@', 0xff, '.', 'c', 'o', 'm'}), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, email, _, err := validator.ValidateSignup("user", tt.email, "password")
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if email != tt.want {
				t.Fatalf("email = %q, want %q", email, tt.want)
			}
		})
	}
}

func TestValidateLoginRequiresOnlyNonEmptyPasswordAndDoesNotTrimIt(t *testing.T) {
	validator := NewUserValidator()

	tests := []struct {
		name      string
		email     string
		password  string
		wantEmail string
		wantError bool
	}{
		{name: "valid", email: " USER@Example.com ", password: "x", wantEmail: "user@example.com"},
		{name: "password spaces are not trimmed", email: "user@example.com", password: "   ", wantEmail: "user@example.com"},
		{name: "empty password", email: "user@example.com", password: "", wantError: true},
		{name: "invalid password utf8", email: "user@example.com", password: string([]byte{0xff}), wantError: true},
		{name: "invalid email", email: "invalid", password: "password", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, password, err := validator.ValidateLogin(tt.email, tt.password)
			if tt.wantError {
				if !errors.Is(err, entity.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if email != tt.wantEmail {
				t.Fatalf("email = %q, want %q", email, tt.wantEmail)
			}
			if password != tt.password {
				t.Fatalf("password was changed: got %q, want %q", password, tt.password)
			}
		})
	}
}
