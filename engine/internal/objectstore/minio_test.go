package objectstore

import "testing"

func TestValidObjectKey(t *testing.T) {
	valid := []string{
		"sites/project_1/deployments/deploy-1/manifest.json",
		"sites/project_1/deployments/deploy-1/files/index.html",
		"sites/project_1/deployments/deploy-1/files/assets/app.js",
	}
	for _, key := range valid {
		if !validObjectKey(key) {
			t.Errorf("expected key to be valid: %q", key)
		}
	}
}

func TestRejectsUnsafeObjectKeys(t *testing.T) {
	invalid := []string{
		"", "/sites/p/deployments/d/manifest.json",
		"sites/p/deployments/d/files/../secret",
		"sites/p/deployments/d/files//index.html",
		"sites/p/deployments/d/files/../../outside",
		"sites/p/deployments/d/files\\windows.txt",
		"other/p/deployments/d/manifest.json",
		"sites/p/deployments/d/private.txt",
	}
	for _, key := range invalid {
		if validObjectKey(key) {
			t.Errorf("expected key to be rejected: %q", key)
		}
	}
}
