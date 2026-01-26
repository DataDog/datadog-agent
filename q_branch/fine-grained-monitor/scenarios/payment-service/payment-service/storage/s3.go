package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storage handles S3 operations
type S3Storage struct {
	client    *s3.Client
	bucket    string
	envPrefix string
}

// NewS3Storage creates a new S3 storage client
func NewS3Storage(ctx context.Context) (*S3Storage, error) {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET environment variable not set")
	}

	// Get DD_ENV or default to "unknown"
	envPrefix := os.Getenv("DD_ENV")
	if envPrefix == "" {
		envPrefix = "unknown"
	}

	// Load AWS configuration from environment variables
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(os.Getenv("AWS_REGION")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return &S3Storage{
		client:    client,
		bucket:    bucket,
		envPrefix: envPrefix,
	}, nil
}

// prefixKey adds the environment prefix to the S3 key
func (s *S3Storage) prefixKey(key string) string {
	return fmt.Sprintf("%s/%s", s.envPrefix, key)
}

// UploadFile uploads a file to S3 and returns the S3 URL
func (s *S3Storage) UploadFile(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	// Add environment prefix to key
	fullKey := s.prefixKey(key)

	// Read the entire body into memory for S3 upload
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, body)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Upload to S3
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(fullKey),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Return S3 URL with full key
	return fmt.Sprintf("s3://%s/%s", s.bucket, fullKey), nil
}

// DeleteFile deletes a file from S3
func (s *S3Storage) DeleteFile(ctx context.Context, key string) error {
	// Add environment prefix to key
	fullKey := s.prefixKey(key)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}
	return nil
}

// GetFile retrieves a file from S3
func (s *S3Storage) GetFile(ctx context.Context, key string) (io.ReadCloser, string, error) {
	// Add environment prefix to key
	fullKey := s.prefixKey(key)

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get from S3: %w", err)
	}

	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	return result.Body, contentType, nil
}

// GetS3KeyFromPath extracts the S3 key from a full S3 path (s3://bucket/env/key)
// and strips the environment prefix to return just the logical key
func GetS3KeyFromPath(s3Path string) string {
	// Extract key from s3://bucket/env/photos/123.jpg format
	if len(s3Path) > 5 && s3Path[:5] == "s3://" {
		// Find the first slash after s3:// (after bucket name)
		idx := 5
		for i := idx; i < len(s3Path); i++ {
			if s3Path[i] == '/' {
				fullKey := s3Path[i+1:]
				// Strip environment prefix (first path segment)
				// e.g., "gensim-dogfeed/photos/123.jpg" -> "photos/123.jpg"
				for j := 0; j < len(fullKey); j++ {
					if fullKey[j] == '/' {
						return fullKey[j+1:]
					}
				}
				return fullKey
			}
		}
	}
	return s3Path
}

// DetectContentType determines the content type based on file extension
func DetectContentType(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".doc", ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls", ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt", ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".txt":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	default:
		return "application/octet-stream"
	}
}
