package analysis

import (
	"path/filepath"
	"testing"
)

func TestIgnorePolicyIgnoresTestdataByDefault(t *testing.T) {
	policy := NewIgnorePolicy(nil)
	root := filepath.Join("/tmp", "workspace")

	if !policy.ShouldIgnore(root, filepath.Join(root, "testdata"), true) {
		t.Fatal("expected testdata directory to be ignored by default")
	}
	if !policy.ShouldIgnore(root, filepath.Join(root, "testdata", "fixture.json"), false) {
		t.Fatal("expected files under testdata to be ignored by default")
	}
}
