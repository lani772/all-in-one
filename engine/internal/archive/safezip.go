package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrUnsafeArchive = errors.New("archive contains an unsafe path or file type")
	ErrArchiveTooLarge = errors.New("archive exceeds configured extraction limits")
)

type Limits struct {
	MaxArchiveBytes int64
	MaxEntries int
	MaxFileBytes int64
	MaxExpandedBytes int64
	MaxCompressionRatio float64
}

func (l Limits) defaults() Limits {
	if l.MaxArchiveBytes <= 0 { l.MaxArchiveBytes = 20 << 20 }
	if l.MaxEntries <= 0 { l.MaxEntries = 10000 }
	if l.MaxFileBytes <= 0 { l.MaxFileBytes = 100 << 20 }
	if l.MaxExpandedBytes <= 0 { l.MaxExpandedBytes = 500 << 20 }
	if l.MaxCompressionRatio <= 0 { l.MaxCompressionRatio = 100 }
	return l
}

func SafeExtract(archivePath, destination string, limits Limits) error {
	return SafeExtractContext(context.Background(), archivePath, destination, limits)
}

func SafeExtractContext(ctx context.Context, archivePath, destination string, limits Limits) error {
	limits = limits.defaults()
	archiveInfo, err := os.Lstat(archivePath)
	if err != nil { return err }
	if !archiveInfo.Mode().IsRegular() || archiveInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: source archive must be a regular file", ErrUnsafeArchive)
	}
	if archiveInfo.Size() < 1 || archiveInfo.Size() > limits.MaxArchiveBytes {
		return ErrArchiveTooLarge
	}
	zr, err := zip.OpenReader(archivePath)
	if err != nil { return fmt.Errorf("open source ZIP: %w", err) }
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > limits.MaxEntries { return ErrArchiveTooLarge }

	root, err := filepath.Abs(destination)
	if err != nil { return err }
	if err := os.Mkdir(root, 0700); err != nil {
		return fmt.Errorf("create fresh extraction directory: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: invalid extraction root", ErrUnsafeArchive)
	}

	var total int64
	seen := make(map[string]struct{}, len(zr.File))
	for _, entry := range zr.File {
		if err := ctx.Err(); err != nil { return err }
		name, err := cleanEntryName(entry.Name)
		if err != nil { return err }
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%w: duplicate entry %q", ErrUnsafeArchive, name)
		}
		seen[name] = struct{}{}

		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsDir() && !mode.IsRegular()) {
			return fmt.Errorf("%w: unsupported entry type %q", ErrUnsafeArchive, name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		rel, err := filepath.Rel(root, target)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("%w: entry escapes extraction root", ErrUnsafeArchive)
		}
		if mode.IsDir() || strings.HasSuffix(entry.Name, "/") {
			if err := os.MkdirAll(target, 0700); err != nil { return err }
			continue
		}

		if entry.UncompressedSize64 > uint64(limits.MaxFileBytes) ||
			entry.UncompressedSize64 > uint64(limits.MaxExpandedBytes) {
			return ErrArchiveTooLarge
		}
		compressed := entry.CompressedSize64
		if entry.UncompressedSize64 > 0 && compressed == 0 {
			return fmt.Errorf("%w: invalid compressed size for %q", ErrUnsafeArchive, name)
		}
		if compressed > 0 && float64(entry.UncompressedSize64)/float64(compressed) > limits.MaxCompressionRatio {
			return fmt.Errorf("%w: compression ratio exceeded for %q", ErrArchiveTooLarge, name)
		}
		if total > limits.MaxExpandedBytes-int64(entry.UncompressedSize64) {
			return ErrArchiveTooLarge
		}
		parent := filepath.Dir(target)
		if err := os.MkdirAll(parent, 0700); err != nil { return err }
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil { return fmt.Errorf("create extracted file %q: %w", name, err) }
		source, err := entry.Open()
		if err != nil { file.Close(); os.Remove(target); return err }
		written, copyErr := copyContext(ctx, file, source, limits.MaxFileBytes+1)
		sourceErr := source.Close()
		fileErr := file.Close()
		if copyErr != nil { os.Remove(target); return copyErr }
		if sourceErr != nil { os.Remove(target); return sourceErr }
		if fileErr != nil { os.Remove(target); return fileErr }
		if written != int64(entry.UncompressedSize64) || written > limits.MaxFileBytes {
			os.Remove(target)
			if written > limits.MaxFileBytes { return ErrArchiveTooLarge }
			return fmt.Errorf("%w: declared and extracted sizes differ for %q", ErrUnsafeArchive, name)
		}
		total += written
	}
	return nil
}

func cleanEntryName(raw string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\\\x00:") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: invalid ZIP entry name", ErrUnsafeArchive)
	}
	name := strings.TrimSuffix(raw, "/")
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || !fs.ValidPath(name) {
		return "", fmt.Errorf("%w: invalid ZIP entry path %q", ErrUnsafeArchive, raw)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: invalid ZIP path segment", ErrUnsafeArchive)
		}
	}
	return name, nil
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	buffer := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil { return total, err }
		n, readErr := src.Read(buffer)
		if n > 0 {
			if total > maxBytes-int64(n) { return total + int64(n), ErrArchiveTooLarge }
			written, writeErr := dst.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil { return total, writeErr }
			if written != n { return total, io.ErrShortWrite }
		}
		if readErr == io.EOF { return total, nil }
		if readErr != nil { return total, readErr }
	}
}
