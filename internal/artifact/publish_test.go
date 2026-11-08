package artifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type memoryObjects struct {
	values map[string][]byte
	failOn string
}

func (m *memoryObjects) Put(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	if key == m.failOn {
		return errors.New("simulated object-store failure")
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return errors.New("size mismatch")
	}
	if m.values == nil {
		m.values = map[string][]byte{}
	}
	m.values[key] = data
	return nil
}
func (m *memoryObjects) Delete(_ context.Context, keys []string) error {
	for _, key := range keys {
		delete(m.values, key)
	}
	return nil
}

type memoryRepo struct {
	called      bool
	owner       string
	manifest    Manifest
	manifestKey string
	err         error
}

func (m *memoryRepo) CommitDeployment(_ context.Context, owner string, manifest Manifest, key string) error {
	m.called, m.owner, m.manifest, m.manifestKey = true, owner, manifest, key
	return m.err
}

func TestPublisherUploadsImmutableFilesAndCommitsMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/index.html", "hello")
	objects, repo := &memoryObjects{}, &memoryRepo{}
	publisher := Publisher{Objects: objects, Repository: repo, NewID: func() (string, error) { return "deploy_01", nil }, Now: func() time.Time { return time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC) }}
	manifest, err := publisher.Publish(context.Background(), "owner_1", "project_1", "build_1", root)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.called || repo.owner != "owner_1" {
		t.Fatalf("repository not called with owner: %+v", repo)
	}
	if manifest.DeploymentID != "deploy_01" || len(manifest.Files) != 1 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if !bytes.Equal(objects.values[manifest.Files[0].ObjectKey], []byte("hello")) {
		t.Fatal("file object was not uploaded")
	}
	if _, ok := objects.values[repo.manifestKey]; !ok {
		t.Fatal("manifest object was not uploaded")
	}
	if !strings.HasSuffix(repo.manifestKey, "/manifest.json") {
		t.Fatalf("unexpected manifest key %s", repo.manifestKey)
	}
}

func TestPublisherRetainsObjectsWhenMetadataCommitIsAmbiguous(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/index.html", "hello")
	objects := &memoryObjects{}
	repo := &memoryRepo{err: errors.New("database unavailable")}
	publisher := Publisher{Objects: objects, Repository: repo, NewID: func() (string, error) { return "deploy_02", nil }}
	if _, err := publisher.Publish(context.Background(), "owner_1", "project_1", "build_1", root); err == nil {
		t.Fatal("expected commit error")
	}
	if len(objects.values) != 2 {
		t.Fatalf("objects must be retained for commit reconcilation, got %d", len(objects.values))
	}
}

func TestPublisherRollsBackEarlierFilesOnObjectFailure(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/a.html", "a")
	writeFile(t, root+"/b.html", "b")
	objects := &memoryObjects{failOn: "sites/project_1/deployments/deploy_03/files/b.html"}
	repo := &memoryRepo{}
	publisher := Publisher{Objects: objects, Repository: repo, NewID: func() (string, error) { return "deploy_03", nil }}
	if _, err := publisher.Publish(context.Background(), "owner_1", "project_1", "build_1", root); err == nil {
		t.Fatal("expected upload error")
	}
	if len(objects.values) != 0 {
		t.Fatalf("objects not rolled back: %v", objects.values)
	}
	if repo.called {
		t.Fatal("metadata must not commit after upload failure")
	}
}
