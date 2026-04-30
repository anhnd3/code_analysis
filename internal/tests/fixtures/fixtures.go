package fixtures

import (
	"path/filepath"
	"runtime"
	"testing"
)

func WorkspacePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve fixture helper path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), name))
}
