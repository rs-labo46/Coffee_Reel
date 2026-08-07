package validator

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"coffee-reel/entity"
	"coffee-reel/usecase"
)

const (
	defaultVideoListLimit           = 20
	maxVideoListLimit               = 100
	maxVideoTitleLength             = 100
	maxVideoDescriptionLength       = 1000
	maxVideoDeclaredSizeBytes int64 = 30_000_000
)

type VideoValidatorConfig struct {
	IdempotencyKeyMaxBytes int
}

type IVideoValidator interface {
	ValidateStartUpload(
		title,
		description,
		category,
		contentType string,
		declaredSize int64,
	) (usecase.StartUploadInput, error)

	ValidateIdempotencyKey(idempotencyKey string) (string, error)
	ValidateVideoID(rawVideoID string) (uint64, error)

	ValidatePublicListQuery(
		rawTitle string,
		titleSpecified bool,
		rawCategory string,
		categorySpecified bool,
		rawLimit string,
		rawCursor string,
	) (usecase.PublicVideoListInput, error)

	ValidateListQuery(
		rawLimit,
		rawCursor string,
	) (usecase.VideoListInput, error)
}

type videoValidator struct {
	idempotencyKeyMaxBytes int
}

func NewVideoValidator(config VideoValidatorConfig) (IVideoValidator, error) {
	if config.IdempotencyKeyMaxBytes < 1 {
		return nil, fmt.Errorf(
			"video validator configuration is invalid",
		)
	}

	return &videoValidator{
		idempotencyKeyMaxBytes: config.IdempotencyKeyMaxBytes,
	}, nil
}

func (v *videoValidator) ValidateStartUpload(title, description, category, contentType string, declaredSize int64) (usecase.StartUploadInput, error) {
	if !utf8.ValidString(title) ||
		!utf8.ValidString(description) ||
		!utf8.ValidString(category) ||
		!utf8.ValidString(contentType) {
		return usecase.StartUploadInput{}, entity.ErrInvalidInput
	}

	title = strings.TrimSpace(title)
	titleLength := utf8.RuneCountInString(title)
	if titleLength < 1 || titleLength > maxVideoTitleLength {
		return usecase.StartUploadInput{}, entity.ErrInvalidInput
	}

	if utf8.RuneCountInString(description) >
		maxVideoDescriptionLength {
		return usecase.StartUploadInput{}, entity.ErrInvalidInput
	}

	categoryCode := entity.CategoryCode(category)
	if !categoryCode.IsValid() {
		return usecase.StartUploadInput{}, entity.ErrInvalidInput
	}

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType != "video/mp4" &&
		contentType != "video/quicktime" {
		return usecase.StartUploadInput{}, entity.ErrInvalidInput
	}

	if declaredSize < 1 ||
		declaredSize > maxVideoDeclaredSizeBytes {
		return usecase.StartUploadInput{}, entity.ErrInvalidInput
	}

	return usecase.StartUploadInput{
		Title:        title,
		Description:  description,
		Category:     categoryCode,
		ContentType:  contentType,
		DeclaredSize: declaredSize,
	}, nil
}

func (v *videoValidator) ValidateIdempotencyKey(idempotencyKey string) (string, error) {
	if !utf8.ValidString(idempotencyKey) ||
		idempotencyKey == "" ||
		len(idempotencyKey) > v.idempotencyKeyMaxBytes {
		return "", entity.ErrInvalidInput
	}

	for _, r := range idempotencyKey {
		if unicode.IsSpace(r) {
			return "", entity.ErrInvalidInput
		}
	}

	return idempotencyKey, nil
}

func (v *videoValidator) ValidateVideoID(rawVideoID string) (uint64, error) {
	if rawVideoID == "" ||
		strings.HasPrefix(rawVideoID, "-") {
		return 0, entity.ErrInvalidInput
	}

	videoID, err := strconv.ParseUint(rawVideoID, 10, 64)
	if err != nil ||
		videoID == 0 ||
		videoID > math.MaxInt64 {
		return 0, entity.ErrInvalidInput
	}

	return videoID, nil
}

func (v *videoValidator) ValidatePublicListQuery(
	rawTitle string,
	titleSpecified bool,
	rawCategory string,
	categorySpecified bool,
	rawLimit string,
	rawCursor string,
) (usecase.PublicVideoListInput, error) {
	input := usecase.PublicVideoListInput{}

	if titleSpecified {
		if !utf8.ValidString(rawTitle) {
			return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
		}
		input.Title = strings.TrimSpace(rawTitle)
		length := utf8.RuneCountInString(input.Title)
		if length < 1 || length > maxVideoTitleLength {
			return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
		}
	}

	if categorySpecified {
		if !utf8.ValidString(rawCategory) {
			return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
		}
		category := entity.CategoryCode(rawCategory)
		if !category.IsValid() {
			return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
		}
		input.Category = &category
	}

	limit, err := validateVideoListLimit(rawLimit)
	if err != nil {
		return usecase.PublicVideoListInput{}, err
	}
	input.Limit = limit

	if rawCursor == "" {
		return input, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil {
		return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
	}

	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()

	var cursor usecase.PublicVideoCursor
	if err := decoder.Decode(&cursor); err != nil {
		return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
	}

	if !cursor.ResultType.IsValid() ||
		cursor.CreatedAt.IsZero() ||
		cursor.ID == 0 ||
		cursor.ID > math.MaxInt64 ||
		!isLowerHexSHA256(cursor.FilterHash) {
		return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
	}

	if cursor.ResultType == usecase.PublicSearchSimilar {
		if cursor.Similarity < 0.6 || cursor.Similarity > 1 {
			return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
		}
	} else if cursor.Similarity != 0 {
		return usecase.PublicVideoListInput{}, entity.ErrInvalidInput
	}

	input.Cursor = &cursor
	return input, nil
}

func validateVideoListLimit(rawLimit string) (int, error) {
	limit := defaultVideoListLimit
	if rawLimit == "" {
		return limit, nil
	}

	parsedLimit, err := strconv.Atoi(rawLimit)
	if err != nil || parsedLimit < 1 || parsedLimit > maxVideoListLimit {
		return 0, entity.ErrInvalidInput
	}
	return parsedLimit, nil
}

func isLowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (v *videoValidator) ValidateListQuery(rawLimit, rawCursor string) (usecase.VideoListInput, error) {
	limit, err := validateVideoListLimit(rawLimit)
	if err != nil {
		return usecase.VideoListInput{}, err
	}

	input := usecase.VideoListInput{
		Limit: limit,
	}

	if rawCursor == "" {
		return input, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(
		rawCursor,
	)
	if err != nil {
		return usecase.VideoListInput{},
			entity.ErrInvalidInput
	}

	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()

	var cursor usecase.VideoCursor
	if err := decoder.Decode(&cursor); err != nil {
		return usecase.VideoListInput{},
			entity.ErrInvalidInput
	}

	if err := ensureJSONEOF(decoder); err != nil {
		return usecase.VideoListInput{},
			entity.ErrInvalidInput
	}

	if cursor.CreatedAt.IsZero() ||
		cursor.ID == 0 ||
		cursor.ID > math.MaxInt64 {
		return usecase.VideoListInput{},
			entity.ErrInvalidInput
	}

	input.Cursor = &cursor

	return input, nil
}
