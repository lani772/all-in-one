package deployment

import (
	"testing"

	"github.com/lani772/all-in-one/engine/internal/artifact"
)

func validManifest() artifact.Manifest {
	return artifact.Manifest{
		Version: 1, ProjectID: "project1", DeploymentID: "deploy1", BuildID: "build1",
		CreatedAt: "2026-11-07T09:00:00Z", TotalBytes: 3,
		Files: []artifact.File{{
			Path: "index.html", Size: 3,
			SHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ContentType: "text/html; charset=utf-8",
			ObjectKey:   "sites/project1/deployments/deploy1/files/index.html",
		}},
	}
}

func TestValidateCommitAcceptsConsistentManifest(t *testing.T) {
	m := validManifest()
	if err := validateCommit("owner1", m, "sites/project1/deployments/deploy1/manifest.json"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCommitRejectsPathTraversal(t *testing.T) {
	m := validManifest()
	m.Files[0].Path = "../secret"
	if err := validateCommit("owner1", m, "sites/project1/deployments/deploy1/manifest.json"); err == nil {
		t.Fatal("expected traversal path rejection")
	}
}

func TestValidateCommitRejectsHashAndSizeMismatch(t *testing.T) {
	m := validManifest()
	m.Files[0].SHA256 = "not-a-hash"
	if err := validateCommit("owner1", m, "sites/project1/deployments/deploy1/manifest.json"); err == nil {
		t.Fatal("expected invalid hash rejection")
	}
	m = validManifest()
	m.TotalBytes = 99
	if err := validateCommit("owner1", m, "sites/project1/deployments/deploy1/manifest.json"); err == nil {
		t.Fatal("expected total size mismatch rejection")
	}
}
