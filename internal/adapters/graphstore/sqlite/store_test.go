package sqlite

import (
	"path/filepath"
	"testing"

	"analysis-module/internal/domain/graph"
)

func TestSaveAndLoadSnapshot(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "analysis.sqlite"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	snapshot := graph.GraphSnapshot{
		ID:          "snap_demo",
		WorkspaceID: "ws_demo",
		Nodes:       []graph.Node{{ID: "n1", Kind: graph.NodeSymbol, CanonicalName: "demo.Handle", SnapshotID: "snap_demo"}},
		Edges:       []graph.Edge{{ID: "e1", Kind: graph.EdgeCalls, From: "n1", To: "n1", SnapshotID: "snap_demo"}},
	}
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	loaded, err := store.GetSnapshot("snap_demo")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if loaded.ID != "snap_demo" {
		t.Fatalf("unexpected snapshot id: %s", loaded.ID)
	}
}
