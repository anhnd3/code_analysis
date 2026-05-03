package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"analysis-module/internal/indexer"
)

func TestSaveJSONWritesSnapshotScopedFile(t *testing.T) {
	root := t.TempDir()
	store := NewArtifactStore(root)

	ref, err := store.SaveJSON(
		"ws_demo",
		"snap_demo",
		"test.json",
		indexer.TypeFactsIndex,
		map[string]string{"key": "value"},
	)
	if err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	expectedPath := filepath.Join(root, "workspaces", "ws_demo", "snapshots", "snap_demo", "test.json")
	if ref.Path != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, ref.Path)
	}

	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload["key"] != "value" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestSaveJSONWritesWorkspaceScopedFileWhenSnapshotEmpty(t *testing.T) {
	root := t.TempDir()
	store := NewArtifactStore(root)

	ref, err := store.SaveJSON(
		"ws_demo",
		"",
		"workspace_manifest.json",
		indexer.TypeWorkspaceManifest,
		map[string]string{"workspace": "demo"},
	)
	if err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	expectedPath := filepath.Join(root, "workspaces", "ws_demo", "workspace_manifest.json")
	if ref.Path != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, ref.Path)
	}

	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestSaveTextWritesSnapshotScopedFile(t *testing.T) {
	root := t.TempDir()
	store := NewArtifactStore(root)

	ref, err := store.SaveText(
		"ws_demo",
		"snap_demo",
		"flow.md",
		indexer.TypeReviewMarkdown,
		"hello",
	)
	if err != nil {
		t.Fatalf("SaveText: %v", err)
	}

	expectedPath := filepath.Join(root, "workspaces", "ws_demo", "snapshots", "snap_demo", "flow.md")
	if ref.Path != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, ref.Path)
	}

	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected body: %q", string(data))
	}
}
