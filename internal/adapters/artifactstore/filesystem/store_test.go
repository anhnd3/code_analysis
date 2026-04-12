package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"analysis-module/internal/domain/graph"
)

func TestSaveGraphWritesJSONLFiles(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	refs, err := store.SaveGraph("ws_demo", "snap_demo", []graph.Node{{ID: "n1"}}, []graph.Edge{{ID: "e1"}})
	if err != nil {
		t.Fatalf("save graph: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces", "ws_demo", "snapshots", "snap_demo", "graph_nodes.jsonl")); err != nil {
		t.Fatalf("nodes artifact missing: %v", err)
	}
}
