package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

type MinIOConfig struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	UseTLS    bool
}

type MinIOStore struct {
	client *minio.Client
	bucket string
}

func NewMinIOStore(ctx context.Context, cfg MinIOConfig) (*MinIOStore, error) {
	if cfg.Endpoint == "" || strings.Contains(cfg.Endpoint, "://") ||
		strings.ContainsAny(cfg.Endpoint, "/?#") {
		return nil, errors.New("S3 endpoint must be a hostname:port without a URL scheme or path")
	}
	if !bucketPattern.MatchString(cfg.Bucket) || strings.Contains(cfg.Bucket, "..") {
		return nil, errors.New("S3 bucket name is invalid")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("S3 credentials are required")
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseTLS,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check private artifact bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("artifact bucket %q does not exist; provision it as private before starting the engine", cfg.Bucket)
	}
	return &MinIOStore{client: client, bucket: cfg.Bucket}, nil
}

func (s *MinIOStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if s == nil || s.client == nil || body == nil || size < 0 || contentType == "" {
		return errors.New("invalid object upload")
	}
	if !validObjectKey(key) {
		return errors.New("unsafe artifact object key")
	}
	counted := &countingReader{reader: body}
	_, err := s.client.PutObject(ctx, s.bucket, key, counted, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("put private artifact object: %w", err)
	}
	if counted.bytesRead != size {
		return fmt.Errorf("object store consumed %d bytes; expected %d", counted.bytesRead, size)
	}
	return nil
}

func (s *MinIOStore) Delete(ctx context.Context, keys []string) error {
	if s == nil || s.client == nil {
		return errors.New("object store is not configured")
	}
	var failures []error
	for _, key := range keys {
		if !validObjectKey(key) {
			failures = append(failures, fmt.Errorf("refusing unsafe object key %q", key))
			continue
		}
		if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
			failures = append(failures, fmt.Errorf("delete %q: %w", key, err))
		}
	}
	return errors.Join(failures...)
}

func validObjectKey(key string) bool {
	if key == "" || strings.Contains(key, "\\") || path.IsAbs(key) || path.Clean(key) != key || !fs.ValidPath(key) {
		return false
	}
	parts := strings.Split(key, "/")
	if len(parts) < 5 || parts[0] != "sites" || !validID(parts[1]) || parts[2] != "deployments" || !validID(parts[3]) {
		return false
	}
	if len(parts) == 5 && parts[4] == "manifest.json" {
		return true
	}
	if len(parts) < 6 || parts[4] != "files" {
		return false
	}
	for _, part := range parts[5:] {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || (i > 0 && (r == '-' || r == '_')) {
			continue
		}
		return false
	}
	return true
}

type countingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}
