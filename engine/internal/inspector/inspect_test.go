package inspector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDetectsReactVite(t *testing.T) {
	root := t.TempDir()
	data := `{"name":"demo","scripts":{"build":"vite build"},"dependencies":{"react":"^19","vite":"^6"}}`
	if err := os.WriteFile(filepath.Join(root,"package.json"), []byte(data), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root,"pnpm-lock.yaml"), []byte("lockfileVersion: '9'"), 0600); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(root,"src"),0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root,"src","main.tsx"), []byte(""),0600); err != nil { t.Fatal(err) }
	got, err := Inspect(root); if err != nil { t.Fatal(err) }
	if got.Framework != "react-vite" || got.PackageManager != "pnpm" || got.BuildScript != "vite build" { t.Fatalf("unexpected result: %+v",got) }
}

func TestInspectDetectsNextRuntimeRequirement(t *testing.T) {
	root := t.TempDir()
	data := `{"dependencies":{"next":"^15","react":"^19"}}`
	if err := os.WriteFile(filepath.Join(root,"package.json"), []byte(data),0600); err != nil { t.Fatal(err) }
	got, err := Inspect(root); if err != nil { t.Fatal(err) }
	if got.Framework != "nextjs" || got.DeploymentTarget != "node-runtime-required" { t.Fatalf("unexpected result: %+v",got) }
}

func TestInspectRejectsInvalidPackageJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root,"package.json"), []byte("{"),0600); err != nil { t.Fatal(err) }
	if _, err := Inspect(root); err == nil { t.Fatal("expected invalid package JSON error") }
}
