package artifact

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ObjectStore interface {
	// Put must consume exactly size bytes and write to a private, non-public object bucket.
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, keys []string) error
}

type DeploymentRepository interface {
	// CommitDeployment must atomically persist the manifest and file rows and switch
	// the project's current deployment pointer. It must verify project ownership.
	CommitDeployment(ctx context.Context, ownerID string, manifest Manifest, manifestKey string) error
}

type Publisher struct {
	Objects    ObjectStore
	Repository DeploymentRepository
	Limits     Limits
	NewID      func() (string, error)
	Now        func() time.Time
}

func (p Publisher) Publish(ctx context.Context, ownerID, projectID, buildID, outputDir string) (Manifest, error) {
	if p.Objects == nil || p.Repository == nil {
		return Manifest{}, errors.New("artifact publisher dependencies are not configured")
	}
	if !validIdentifier(ownerID) || !validIdentifier(projectID) || !validIdentifier(buildID) {
		return Manifest{}, ErrInvalidIdentifier
	}
	newID := p.NewID
	if newID == nil {
		newID = randomID
	}
	deploymentID, err := newID()
	if err != nil {
		return Manifest{}, fmt.Errorf("generate deployment id: %w", err)
	}
	if !validIdentifier(deploymentID) {
		return Manifest{}, ErrInvalidIdentifier
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	manifest, err := BuildManifest(outputDir, projectID, deploymentID, buildID, p.Limits)
	if err != nil {
		return Manifest{}, err
	}
	manifest.CreatedAt = now().UTC().Format(time.RFC3339Nano)

	uploaded := make([]string, 0, len(manifest.Files)+1)
	rollback := func(cause error) error {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if len(uploaded) == 0 {
			return cause
		}
		if cleanupErr := p.Objects.Delete(cleanupCtx, uploaded); cleanupErr != nil {
			return fmt.Errorf("%w (artifact cleanup also failed: %v)", cause, cleanupErr)
		}
		return cause
	}

	for _, item := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return Manifest{}, rollback(err)
		}
		fullPath := filepath.Join(outputDir, filepath.FromSlash(item.Path))
		file, err := openVerifiedFile(outputDir, fullPath, item.Size)
		if err != nil {
			return Manifest{}, rollback(err)
		}
		reader := &digestReader{source: file, digest: sha256.New()}
		// Register the key before Put: an object store may have persisted a
		// partial multipart upload even when it returns an error.
		uploaded = append(uploaded, item.ObjectKey)
		err = p.Objects.Put(ctx, item.ObjectKey, reader, item.Size, item.ContentType)
		closeErr := file.Close()
		if err != nil {
			return Manifest{}, rollback(fmt.Errorf("upload artifact %q: %w", item.Path, err))
		}
		if closeErr != nil {
			return Manifest{}, rollback(closeErr)
		}
		if reader.read != item.Size || reader.sum() != item.SHA256 {
			return Manifest{}, rollback(fmt.Errorf("%w: artifact %q changed during upload", ErrUnsafeArtifact, item.Path))
		}
	}

	manifestKey := filepath.ToSlash(filepath.Join("sites", projectID, "deployments", deploymentID, "manifest.json"))
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, rollback(err)
	}
	uploaded = append(uploaded, manifestKey)
	if err := p.Objects.Put(ctx, manifestKey, bytes.NewReader(manifestBytes), int64(len(manifestBytes)), "application/json"); err != nil {
		return Manifest{}, rollback(fmt.Errorf("upload deployment manifest: %w", err))
	}
	if err := p.Repository.CommitDeployment(ctx, ownerID, manifest, manifestKey); err != nil {
		// A database COMMIT error can be ambiguous: the server may have
		// committed before the connection failed. Keep immutable objects in
		// place; an orphan sweeper must check DB references before deleting.
		return Manifest{}, fmt.Errorf("commit deployment metadata (objects retained for reconciliation): %w", err)
	}
	return manifest, nil
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func openVerifiedFile(root, fullPath string, expectedSize int64) (*os.File, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, ErrUnsafeArtifact
	}
	rootInfo, err := os.Lstat(absRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafeArtifact
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil || resolvedRoot != absRoot {
		return nil, ErrUnsafeArtifact
	}
	current := absRoot
	for _, segment := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		component, statErr := os.Lstat(current)
		if statErr != nil {
			return nil, statErr
		}
		if component.Mode()&os.ModeSymlink != 0 {
			return nil, ErrUnsafeArtifact
		}
	}
	before, err := os.Lstat(absPath)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() != expectedSize {
		return nil, ErrUnsafeArtifact
	}
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !after.Mode().IsRegular() || after.Size() != expectedSize || !os.SameFile(before, after) {
		f.Close()
		return nil, ErrUnsafeArtifact
	}
	return f, nil
}

type digestReader struct {
	source io.Reader
	digest hash.Hash
	read   int64
}

func (r *digestReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if n > 0 {
		_, _ = r.digest.Write(p[:n])
		r.read += int64(n)
	}
	return n, err
}

func (r *digestReader) sum() string { return hex.EncodeToString(r.digest.Sum(nil)) }
