package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
)

func (s *MinIOStore) DownloadSource(ctx context.Context, key, destination string, maxBytes int64) (int64, error) {
	if s == nil || s.client == nil || !validSourceObjectKey(key) {
		return 0, errors.New("invalid source object key")
	}
	if maxBytes < 1 { return 0, errors.New("source download limit must be positive") }
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil { return 0, fmt.Errorf("stat source archive: %w", err) }
	if info.Size < 1 || info.Size > maxBytes { return 0, errors.New("source archive exceeds configured byte limit") }
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil { return 0, fmt.Errorf("open source archive: %w", err) }
	defer object.Close()
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil { return 0, err }
	ok := false
	defer func() { file.Close(); if !ok { _ = os.Remove(destination) } }()
	n, err := io.Copy(file, io.LimitReader(object, maxBytes+1))
	if err != nil { return n, fmt.Errorf("download source archive: %w", err) }
	if n != info.Size || n > maxBytes { return n, errors.New("downloaded source size does not match object metadata") }
	if err := file.Sync(); err != nil { return n, err }
	if err := file.Close(); err != nil { return n, err }
	ok = true
	return n, nil
}

func sourceObjectKeyPrefix(key string) bool {
	return strings.HasPrefix(key, "sources/")
}
