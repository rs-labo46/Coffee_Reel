package validator

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"coffee-reel/entity"
	"coffee-reel/usecase"
)

const (
	defaultAdminVideoListLimit = 20
	maxAdminVideoListLimit     = 100
	maxAdminVideoReasonLength  = 500
)

type IAdminVideoValidator interface {
	ValidateVideoID(rawVideoID string) (uint64, error)
	ValidateReason(reason string) (string, error)
	ValidateReasonRequest(body io.Reader) (string, error)
	ValidateListQuery(rawLimit string, rawCursor string) (usecase.AdminVideoListInput, error)
}

type adminVideoValidator struct{}

type adminVideoReasonRequest struct {
	Reason string `json:"reason"`
}

func NewAdminVideoValidator() IAdminVideoValidator {
	return &adminVideoValidator{}
}

func (v *adminVideoValidator) ValidateVideoID(rawVideoID string) (uint64, error) {
	if rawVideoID == "" ||
		strings.HasPrefix(rawVideoID, "-") {
		return 0, entity.ErrInvalidInput
	}

	videoID, err := strconv.ParseUint(
		rawVideoID,
		10,
		64,
	)
	if err != nil ||
		videoID == 0 ||
		videoID > math.MaxInt64 {
		return 0, entity.ErrInvalidInput
	}

	return videoID, nil
}

func (v *adminVideoValidator) ValidateReason(reason string) (string, error) {
	if !utf8.ValidString(reason) {
		return "", entity.ErrInvalidInput
	}

	reason = strings.TrimSpace(reason)

	length := utf8.RuneCountInString(reason)
	if length < 1 ||
		length > maxAdminVideoReasonLength {
		return "", entity.ErrInvalidInput
	}

	return reason, nil
}

func (v *adminVideoValidator) ValidateReasonRequest(body io.Reader) (string, error) {
	if body == nil {
		return "", entity.ErrInvalidInput
	}

	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	var request adminVideoReasonRequest
	if err := decoder.Decode(&request); err != nil {
		return "", entity.ErrInvalidInput
	}

	if err := ensureJSONEOF(decoder); err != nil {
		return "", entity.ErrInvalidInput
	}

	return v.ValidateReason(request.Reason)
}

func (v *adminVideoValidator) ValidateListQuery(rawLimit string, rawCursor string) (usecase.AdminVideoListInput, error) {
	limit := defaultAdminVideoListLimit

	if rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil ||
			parsedLimit < 1 ||
			parsedLimit > maxAdminVideoListLimit {
			return usecase.AdminVideoListInput{},
				entity.ErrInvalidInput
		}

		limit = parsedLimit
	}

	input := usecase.AdminVideoListInput{
		Limit: limit,
	}

	if rawCursor == "" {
		return input, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(
		rawCursor,
	)
	if err != nil {
		return usecase.AdminVideoListInput{},
			entity.ErrCursorInvalid
	}

	decoder := json.NewDecoder(
		bytes.NewReader(decoded),
	)
	decoder.DisallowUnknownFields()

	var cursor usecase.AdminVideoCursor
	if err := decoder.Decode(&cursor); err != nil {
		return usecase.AdminVideoListInput{},
			entity.ErrCursorInvalid
	}

	if err := ensureJSONEOF(decoder); err != nil {
		return usecase.AdminVideoListInput{},
			entity.ErrCursorInvalid
	}

	if cursor.CreatedAt.IsZero() ||
		cursor.ID == 0 ||
		cursor.ID > math.MaxInt64 {
		return usecase.AdminVideoListInput{},
			entity.ErrCursorInvalid
	}

	input.Cursor = &cursor

	return input, nil
}
