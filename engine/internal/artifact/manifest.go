package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrInvalidIdentifier = errors.New("artifact identifier is invalid")
	ErrUnsafeArtifact    = errors.New("artifact contains an unsafe file or path")
	ErrArtifactTooLarge  = errors.New("artifact exceeds configured limits")
)

type Limits struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
}

func (l Limits) withDefaults() Limits {
	if l.MaxFiles <= 0 {
		l.MaxFiles = 20_000
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = 100 << 20 // 100 MiB per file
	}
	if l.MaxTotalBytes <= 0 {
		l.MaxTotalBytes = 1 << 30 // 1 GiB per deployment
	}
	return l
}

type File struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	ObjectKey   string `json:"objectKey"`
}

type Manifest struct {
	Version      int    `json:"version"`
	ProjectID    string `json:"projectId"`
	DeploymentID string `json:"deploymentId"`
	BuildID      string `json:"buildId"`
	CreatedAt    string `json:"createdAt"`
	TotalBytes   int64  `json:"totalBytes"`
	Files        []File `json:"files"`
}

func validIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (i > 0 && (r == '-' || r == '_')) {
			continue
		}
		return false
	}
	return true
}

func BuildManifest(root, projectID, deploymentID, buildID string, limits Limits) (Manifest, error) {
	if !validIdentifier(projectID) || !validIdentifier(deploymentID) || !validIdentifier(buildID) {
		return Manifest{}, ErrInvalidIdentifier
	}
	limits = limits.withDefaults()
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("inspect artifact root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, fmt.Errorf("%w: artifact root must be a real directory", ErrUnsafeArtifact)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return Manifest{}, err
	}
	if resolvedRoot != absRoot {
		return Manifest{}, fmt.Errorf("%w: artifact root resolves through a symlink", ErrUnsafeArtifact)
	}

	manifest := Manifest{Version: 1, ProjectID: projectID, DeploymentID: deploymentID, BuildID: buildID, Files: make([]File, 0)}
	err = filepath.WalkDir(absRoot, func(fullPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fullPath == absRoot {
			return nil
		}
		rel, err := filepath.Rel(absRoot, fullPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !filepath.IsLocal(filepath.FromSlash(rel)) || rel == "." || strings.Contains(rel, "\\") {
			return fmt.Errorf("%w: invalid relative path %q", ErrUnsafeArtifact, rel)
		}
		if excludedDirectory(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(fullPath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("%w: non-regular file %q", ErrUnsafeArtifact, rel)
		}
		if info.IsDir() {
			return nil
		}
		if sensitiveFile(rel) {
			return fmt.Errorf("%w: sensitive file %q must not be published", ErrUnsafeArtifact, rel)
		}
		if info.Size() < 0 || info.Size() > limits.MaxFileBytes {
			return ErrArtifactTooLarge
		}
		if len(manifest.Files) >= limits.MaxFiles || manifest.TotalBytes > limits.MaxTotalBytes-info.Size() {
			return ErrArtifactTooLarge
		}
		sha, size, err := hashFile(fullPath, limits.MaxFileBytes)
		if err != nil {
			return err
		}
		if size != info.Size() {
			return fmt.Errorf("%w: file changed while manifest was being built", ErrUnsafeArtifact)
		}
		key := path.Join("sites", projectID, "deployments", deploymentID, "files", rel)
		manifest.Files = append(manifest.Files, File{Path: rel, Size: size, SHA256: sha, ContentType: contentType(rel), ObjectKey: key})
		manifest.TotalBytes += size
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}
	if len(manifest.Files) == 0 {
		return Manifest{}, errors.New("build output contains no publishable files")
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	return manifest, nil
}

func hashFile(filePath string, maxBytes int64) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", n, err
	}
	if n > maxBytes {
		return "", n, ErrArtifactTooLarge
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func excludedDirectory(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		switch strings.ToLower(segment) {
		case ".git", "node_modules", "__macosx":
			return true
		}
	}
	return false
}

func sensitiveFile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	if base == ".env" || strings.HasPrefix(base, ".env.") || base == ".npmrc" || base == ".pypirc" || base == ".netrc" || base == ".git-credentials" {
		return true
	}
	for _, suffix := range []string{".pem", ".key", ".p12", ".pfx", ".jks", ".keystore"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return base == "id_rsa" || base == "id_ed25519"
}

func contentType(rel string) string {
	ext := strings.ToLower(path.Ext(rel))
	known := map[string]string{
		".html": "text/html; charset=utf-8", ".htm": "text/html; charset=utf-8",
		".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8",
		".mjs": "text/javascript; charset=utf-8", ".cjs": "text/javascript; charset=utf-8",
		".json": "application/json", ".webmanifest": "application/manifest+json",
		".svg": "image/svg+xml", ".png": "image/png", ".jpg": "image/jpeg",
		".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
		".ico": "image/x-icon", ".woff": "font/woff", ".woff2": "font/woff2",
		".ttf": "font/ttf", ".otf": "font/otf", ".wasm": "application/wasm",
		".txt": "text/plain; charset=utf-8", ".xml": "application/xml",
		".pdf": "application/pdf", ".mp4": "video/mp4", ".mp3": "audio/mpeg",
	}
	if value, ok := known[ext]; ok {
		return value
	}
	return "application/octet-stream"
}
