package repository

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidateStorageEndpointPreservesSupabaseS3Path(t *testing.T) {
	got, err := validateStorageEndpoint(
		"https://project-ref.storage.supabase.co/storage/v1/s3/",
		true,
	)
	if err != nil {
		t.Fatalf("validateStorageEndpoint: %v", err)
	}

	want := "https://project-ref.storage.supabase.co/storage/v1/s3"
	if got.String() != want {
		t.Fatalf("endpoint = %q, want %q", got.String(), want)
	}
}

func TestObjectStorageRepositoryPresignKeepsSupabaseS3Path(t *testing.T) {
	storage, err := NewObjectStorageRepository(
		context.Background(),
		ObjectStorageConfig{
			Endpoint:        "https://project-ref.storage.supabase.co/storage/v1/s3",
			PresignEndpoint: "https://project-ref.storage.supabase.co/storage/v1/s3",
			Region:          "ap-northeast-1",
			Bucket:          "coffee-reel",
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
			ManagedPrefix:   "videos/",
			ForcePathStyle:  true,
			RequireHTTPS:    true,
		},
	)
	if err != nil {
		t.Fatalf("NewObjectStorageRepository: %v", err)
	}

	target, err := storage.CreateUploadURL(
		context.Background(),
		"videos/source/test.mp4",
		"video/mp4",
		time.Minute,
	)
	if err != nil {
		t.Fatalf("CreateUploadURL: %v", err)
	}

	parsed, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}

	wantPrefix := "/storage/v1/s3/coffee-reel/videos/"
	if !strings.HasPrefix(parsed.Path, wantPrefix) {
		t.Fatalf(
			"presigned URL path = %q, want prefix %q",
			parsed.Path,
			wantPrefix,
		)
	}
}
