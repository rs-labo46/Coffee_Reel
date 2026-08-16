package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"coffee-reel/entity"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	processedVideoContentType = "video/mp4"
	thumbnailContentType      = "image/jpeg"
)

type IObjectStorageRepository interface {
	CheckHealth(ctx context.Context) error
	CreateUploadURL(ctx context.Context, objectKey, contentType string, ttl time.Duration) (UploadTarget, error)
	Exists(ctx context.Context, objectKey string) (bool, error)
	Stat(ctx context.Context, objectKey string) (StoredObjectInfo, error)
	Download(ctx context.Context, objectKey, destinationPath string) error
	UploadProcessed(ctx context.Context, objectKey, sourcePath string) error
	UploadThumbnail(ctx context.Context, objectKey, sourcePath string) error
	CreateReadURL(ctx context.Context, objectKey string, ttl time.Duration) (ReadTarget, error)
	Delete(ctx context.Context, objectKey string) error
	ListManagedObjects(ctx context.Context, cursor *string, limit int32) (ManagedObjectPage, error)
}

type ObjectStorageConfig = entity.ObjectStorageConfig

type UploadTarget struct {
	Method      string
	URL         string
	ContentType string
	ExpiresAt   time.Time
}

type ReadTarget struct {
	URL       string
	ExpiresAt time.Time
}

type StoredObjectInfo struct {
	ObjectKey      string
	SizeBytes      int64
	ContentType    string
	LastModifiedAt time.Time
}

type ManagedObject struct {
	ObjectKey      string
	SizeBytes      int64
	LastModifiedAt time.Time
}

type ManagedObjectPage struct {
	Items      []ManagedObject
	NextCursor *string
	HasMore    bool
}

type objectStorageRepository struct {
	bucket        string
	managedPrefix string
	client        *s3.Client
	presigner     *s3.PresignClient
}

func NewObjectStorageRepository(ctx context.Context, storageConfig ObjectStorageConfig) (IObjectStorageRepository, error) {
	storageConfig.Endpoint = strings.TrimSpace(storageConfig.Endpoint)
	storageConfig.PresignEndpoint = strings.TrimSpace(storageConfig.PresignEndpoint)
	storageConfig.Region = strings.TrimSpace(storageConfig.Region)
	storageConfig.Bucket = strings.TrimSpace(storageConfig.Bucket)
	storageConfig.AccessKeyID = strings.TrimSpace(storageConfig.AccessKeyID)
	storageConfig.SecretAccessKey = strings.TrimSpace(storageConfig.SecretAccessKey)
	storageConfig.ManagedPrefix = normalizeManagedPrefix(storageConfig.ManagedPrefix)
	if storageConfig.Endpoint == "" || storageConfig.Region == "" || storageConfig.Bucket == "" || storageConfig.AccessKeyID == "" || storageConfig.SecretAccessKey == "" || storageConfig.ManagedPrefix == "" {
		return nil, fmt.Errorf("object storage configuration is incomplete")
	}
	if storageConfig.PresignEndpoint == "" {
		storageConfig.PresignEndpoint = storageConfig.Endpoint
	}

	endpointURL, err := validateStorageEndpoint(storageConfig.Endpoint, storageConfig.RequireHTTPS)
	if err != nil {
		return nil, err
	}
	presignEndpointURL, err := validateStorageEndpoint(storageConfig.PresignEndpoint, storageConfig.RequireHTTPS)
	if err != nil {
		return nil, fmt.Errorf("object storage presign endpoint is invalid: %w", err)
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(storageConfig.Region), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(storageConfig.AccessKeyID, storageConfig.SecretAccessKey, "")))
	if err != nil {
		return nil, fmt.Errorf("load object storage configuration: %w", err)
	}

	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpointURL.String())
		options.UsePathStyle = storageConfig.ForcePathStyle
	})
	presignClient := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(presignEndpointURL.String())
		options.UsePathStyle = storageConfig.ForcePathStyle
	})

	return &objectStorageRepository{
		bucket:        storageConfig.Bucket,
		managedPrefix: storageConfig.ManagedPrefix,
		client:        client,
		presigner:     s3.NewPresignClient(presignClient),
	}, nil
}

func (r *objectStorageRepository) CheckHealth(ctx context.Context) error {
	_, err := r.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(r.bucket),
	})
	if err != nil {
		return storageUnavailable("check object storage bucket", err)
	}

	return nil
}

func validateStorageEndpoint(raw string, requireHTTPS bool) (*url.URL, error) {
	endpointURL, err := url.Parse(raw)
	if err != nil || endpointURL.Host == "" || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") {
		return nil, fmt.Errorf("object storage endpoint is invalid")
	}

	if endpointURL.User != nil || endpointURL.RawQuery != "" || endpointURL.Fragment != "" {
		return nil, fmt.Errorf("object storage endpoint must not contain credentials, query, or fragment")
	}

	if requireHTTPS && endpointURL.Scheme != "https" {
		return nil, fmt.Errorf("object storage endpoint must use HTTPS")
	}

	endpointURL.Path = strings.TrimRight(endpointURL.Path, "/")

	return endpointURL, nil
}

func normalizeManagedPrefix(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	if value == "" || strings.Contains(value, "\\") {
		return ""
	}

	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ""
		}
	}

	return value + "/"
}

func (r *objectStorageRepository) CreateUploadURL(ctx context.Context, objectKey, contentType string, ttl time.Duration) (UploadTarget, error) {
	if !r.isManagedObjectKey(objectKey) || (contentType != "video/mp4" && contentType != "video/quicktime") || ttl <= 0 {
		return UploadTarget{}, entity.ErrInvalidInput
	}

	issuedAt := time.Now()
	result, err := r.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return UploadTarget{}, storageUnavailable("presign upload object", err)
	}

	return UploadTarget{
		Method:      "PUT",
		URL:         result.URL,
		ContentType: contentType,
		ExpiresAt:   issuedAt.Add(ttl),
	}, nil
}

func (r *objectStorageRepository) Exists(ctx context.Context, objectKey string) (bool, error) {
	_, err := r.Stat(ctx, objectKey)
	if errors.Is(err, entity.ErrObjectNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *objectStorageRepository) Stat(ctx context.Context, objectKey string) (StoredObjectInfo, error) {
	if !r.isManagedObjectKey(objectKey) {
		return StoredObjectInfo{}, entity.ErrInvalidInput
	}

	result, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isStorageNotFound(err) {
			return StoredObjectInfo{}, entity.ErrObjectNotFound
		}
		return StoredObjectInfo{}, storageUnavailable("stat object", err)
	}

	info := StoredObjectInfo{ObjectKey: objectKey}
	if result.ContentLength != nil {
		info.SizeBytes = *result.ContentLength
	}
	if result.ContentType != nil {
		info.ContentType = *result.ContentType
	}
	if result.LastModified != nil {
		info.LastModifiedAt = *result.LastModified
	}

	return info, nil
}

func (r *objectStorageRepository) Download(ctx context.Context, objectKey, destinationPath string) error {
	if !r.isManagedObjectKey(objectKey) || strings.TrimSpace(destinationPath) == "" {
		return entity.ErrInvalidInput
	}

	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isStorageNotFound(err) {
			return entity.ErrObjectNotFound
		}
		return storageUnavailable("download object", err)
	}
	defer result.Body.Close()

	file, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create download destination: %w", err)
	}
	if _, err := io.Copy(file, result.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(destinationPath)
		return storageUnavailable("write downloaded object", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return fmt.Errorf("close download destination: %w", err)
	}

	return nil
}

func (r *objectStorageRepository) UploadProcessed(ctx context.Context, objectKey, sourcePath string) error {
	return r.upload(ctx, objectKey, processedVideoContentType, sourcePath)
}

func (r *objectStorageRepository) UploadThumbnail(ctx context.Context, objectKey, sourcePath string) error {
	return r.upload(ctx, objectKey, thumbnailContentType, sourcePath)
}

func (r *objectStorageRepository) upload(ctx context.Context, objectKey, contentType, sourcePath string) error {
	if !r.isManagedObjectKey(objectKey) || strings.TrimSpace(sourcePath) == "" {
		return entity.ErrInvalidInput
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open upload source: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat upload source: %w", err)
	}
	if !stat.Mode().IsRegular() || stat.Size() < 1 {
		return entity.ErrInvalidInput
	}

	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentLength: aws.Int64(stat.Size()),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return storageUnavailable("upload object", err)
	}

	return nil
}

func (r *objectStorageRepository) CreateReadURL(ctx context.Context, objectKey string, ttl time.Duration) (ReadTarget, error) {
	if !r.isManagedObjectKey(objectKey) || ttl <= 0 {
		return ReadTarget{}, entity.ErrInvalidInput
	}

	issuedAt := time.Now()
	result, err := r.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(objectKey),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return ReadTarget{}, storageUnavailable("presign read object", err)
	}

	return ReadTarget{
		URL:       result.URL,
		ExpiresAt: issuedAt.Add(ttl),
	}, nil
}

func (r *objectStorageRepository) Delete(ctx context.Context, objectKey string) error {
	if !r.isManagedObjectKey(objectKey) {
		return entity.ErrInvalidInput
	}

	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil && !isStorageNotFound(err) {
		return storageUnavailable("delete object", err)
	}

	return nil
}

func (r *objectStorageRepository) ListManagedObjects(ctx context.Context, cursor *string, limit int32) (ManagedObjectPage, error) {
	if limit < 1 || limit > 1000 || (cursor != nil && (*cursor == "" || *cursor != strings.TrimSpace(*cursor))) {
		return ManagedObjectPage{}, entity.ErrInvalidInput
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(r.bucket),
		Prefix:  aws.String(r.managedPrefix),
		MaxKeys: aws.Int32(limit),
	}
	if cursor != nil {
		input.ContinuationToken = cursor
	}

	result, err := r.client.ListObjectsV2(ctx, input)
	if err != nil {
		return ManagedObjectPage{}, storageUnavailable("list managed objects", err)
	}

	items := make([]ManagedObject, 0, len(result.Contents))
	for _, item := range result.Contents {
		if item.Key == nil || !r.isManagedObjectKey(*item.Key) {
			continue
		}

		managed := ManagedObject{
			ObjectKey: *item.Key,
			SizeBytes: aws.ToInt64(item.Size),
		}
		if item.LastModified != nil {
			managed.LastModifiedAt = *item.LastModified
		}

		items = append(items, managed)
	}

	page := ManagedObjectPage{
		Items:   items,
		HasMore: aws.ToBool(result.IsTruncated),
	}
	if page.HasMore {
		if result.NextContinuationToken == nil || *result.NextContinuationToken == "" {
			return ManagedObjectPage{}, fmt.Errorf("list managed objects continuation token is missing: %w", entity.ErrStorageUnavailable)
		}

		next := *result.NextContinuationToken
		page.NextCursor = &next
	}

	return page, nil
}

func (r *objectStorageRepository) isManagedObjectKey(objectKey string) bool {
	if objectKey == "" || objectKey != strings.TrimSpace(objectKey) || !strings.HasPrefix(objectKey, r.managedPrefix) {
		return false
	}

	relativeKey := strings.TrimPrefix(objectKey, r.managedPrefix)
	if relativeKey == "" || strings.Contains(relativeKey, "\\") {
		return false
	}

	for _, segment := range strings.Split(relativeKey, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}

	return true
}

func storageUnavailable(operation string, cause error) error {
	return fmt.Errorf("%s: %w", operation, errors.Join(entity.ErrStorageUnavailable, cause))
}

func isStorageNotFound(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return true
		case "NoSuchBucket":
			return false
		}
	}

	var responseError *smithyhttp.ResponseError
	return errors.As(err, &responseError) && responseError.HTTPStatusCode() == 404
}
