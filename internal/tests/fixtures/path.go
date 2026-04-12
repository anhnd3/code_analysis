package fixtures

import (
	"os"
	"path/filepath"
	"testing"
)

func WorkspacePath(t *testing.T, name string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	current := wd
	for {
		candidate := filepath.Join(current, "testdata", "workspaces", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("fixture workspace %q not found from %s", name, wd)
		}
		current = parent
	}
}
