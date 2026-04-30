package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveJSONWritesFileCorrectly(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	ref, err := store.SaveJSON("ws_demo", "snap_demo", "test.json", "json_artifact", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Save