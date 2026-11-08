package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func makeZIP(t *testing.T, path string, entries []struct{name, body string; mode os.FileMode}) {
	t.Helper()
	f, err := os.Create(path); if err != nil { t.Fatal(err) }
	w := zip.NewWriter(f)
	for _, item := range entries {
		h := &zip.FileHeader{Name:item.name}
		h.SetMode(item.mode)
		entry, err := w.CreateHeader(h); if err != nil { t.Fatal(err) }
		if _, err := entry.Write([]byte(item.body)); err != nil { t.Fatal(err) }
	}
	if err := w.Close(); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
}

func TestSafeExtractValidProject(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "site.zip")
	makeZIP(t, archivePath, []struct{name, body string; mode os.FileMode}{
		{"index.html", "<h1>hello</h1>", 0600},
		{"assets/app.js", "console.log('ok')", 0600},
	})
	out := filepath.Join(t.TempDir(), "out")
	if err := SafeExtract(archivePath, out, Limits{}); err != nil { t.Fatal(err) }
	if _, err := os.Stat(filepath.Join(out, "assets", "app.js")); err != nil { t.Fatal(err) }
}

func TestSafeExtractRejectsTraversalAndAbsolutePaths(t *testing.T) {
	for _, name := range []string{"../escape.txt", "/absolute.txt", "C:/drive.txt", "a/../../escape"} {
		archivePath := filepath.Join(t.TempDir(), "bad.zip")
		makeZIP(t, archivePath, []struct{name, body string; mode os.FileMode}{{name, "bad", 0600}})
		out := filepath.Join(t.TempDir(), "out")
		if err := SafeExtract(archivePath, out, Limits{}); err == nil { t.Fatalf("expected %q to be rejected", name) }
	}
}

func TestSafeExtractRejectsSymlink(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "link.zip")
	makeZIP(t, archivePath, []struct{name, body string; mode os.FileMode}{{"link", "target", os.ModeSymlink | 0777}})
	if err := SafeExtract(archivePath, filepath.Join(t.TempDir(), "out"), Limits{}); err == nil { t.Fatal("expected symlink rejection") }
}

func TestSafeExtractEnforcesEntryAndExpandedLimits(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "large.zip")
	makeZIP(t, archivePath, []struct{name, body string; mode os.FileMode}{
		{"a.txt", "a", 0600}, {"b.txt", "b", 0600},
	})
	if err := SafeExtract(archivePath, filepath.Join(t.TempDir(), "out1"), Limits{MaxEntries:1}); err == nil { t.Fatal("expected entry limit") }
	if err := SafeExtract(archivePath, filepath.Join(t.TempDir(), "out2"), Limits{MaxExpandedBytes:1}); err == nil { t.Fatal("expected expanded size limit") }
}
