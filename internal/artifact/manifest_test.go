package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildManifestHashesAndSortsFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "z", "app.js"), "console.log('ok')")
	writeFile(t, filepath.Join(root, "index.html"), "<h1>hello</h1>")
	manifest, err := BuildManifest(root, "project_1", "deploy_1", "build_1", Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("want 2 files, got %d", len(manifest.Files))
	}
	if manifest.Files[0].Path != "index.html" || manifest.Files[1].Path != "z/app.js" {
		t.Fatalf("files not sorted: %+v", manifest.Files)
	}
	want := sha256.Sum256([]byte("<h1>hello</h1>"))
	if manifest.Files[0].SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("unexpected digest: %s", manifest.Files[0].SHA256)
	}
	if manifest.TotalBytes != int64(len("<h1>hello</h1>")+len("console.log('ok')")) {
		t.Fatalf("unexpected total: %d", manifest.TotalBytes)
	}
	if manifest.Files[0].ObjectKey != "sites/project_1/deployments/deploy_1/files/index.html" {
		t.Fatalf("unexpected object key: %s", manifest.Files[0].ObjectKey)
	}
}

func TestBuildManifestRejectsSensitiveFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "ok")
	writeFile(t, filepath.Join(root, ".env.production"), "SECRET=value")
	_, err := BuildManifest(root, "p1", "d1", "b1", Limits{})
	if !errors.Is(err, ErrUnsafeArtifact) {
		t.Fatalf("want ErrUnsafeArtifact, got %v", err)
	}
}

func TestBuildManifestRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(root, "secret.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := BuildManifest(root, "p1", "d1", "b1", Limits{})
	if !errors.Is(err, ErrUnsafeArtifact) {
		t.Fatalf("want ErrUnsafeArtifact, got %v", err)
	}
}

func TestBuildManifestEnforcesLimitsAndRejectsEmptyOutput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "12345")
	_, err := BuildManifest(root, "p1", "d1", "b1", Limits{MaxFiles: 1, MaxFileBytes: 4, MaxTotalBytes: 4})
	if !errors.Is(err, ErrArtifactTooLarge) {
		t.Fatalf("want size error, got %v", err)
	}
	_, err = BuildManifest(t.TempDir(), "p1", "d1", "b1", Limits{})
	if err == nil {
		t.Fatal("empty output should fail")
	}
}

func TestBuildManifestRejectsUnsafeIdentifiers(t *testing.T) {
	_, err := BuildManifest(t.TempDir(), "../other", "d1", "b1", Limits{})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("want identifier error, got %v", err)
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestContentTypeIsDeterministicForFrontendAssets(t *testing.T) {
	if got := contentType("assets/app.js"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected JS content type: %s", got)
	}
	if got := contentType("assets/logo.svg"); got != "image/svg+xml" {
		t.Fatalf("unexpected SVG content type: %s", got)
	}
}
