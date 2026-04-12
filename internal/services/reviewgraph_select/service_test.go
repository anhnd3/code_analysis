package reviewgraph_select

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/services/reviewgraph_paths"
)

func TestSelectManualFilePrioritizesEntrypointThenPublicAPI(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "review_graph.sqlite")
	store, err := reviewsqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	snapshotID := "snap_demo"
	nodes := []reviewgraph.Node{
		{ID: "file:repo:src/app.go", SnapshotID: snapshotID, Repo: "repo", Language: "go", Kind: reviewgraph.NodeFile, Symbol: "src/app.go", FilePath: "src/app.go", NodeRole: reviewgraph.RoleNormal},
		{ID: "go:repo:src/app.go:function:pkg.Handle", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "pkg.Handle", FilePath: "src/app.go", StartLine: 1, EndLine: 10, NodeRole: reviewgraph.RoleEntrypoint},
		{ID: "go:repo:src/app.go:function:pkg.Helper", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "pkg.Helper", FilePath: "src/app.go", StartLine: 12, EndLine: 20, NodeRole: reviewgraph.RolePublicAPI},
	}
	if err := store.ReplaceSnapshot(snapshotID, nodes, nil, nil); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}
	service := New(reviewgraph_paths.New(t.TempDir()))
	outPath := filepath.Join(t.TempDir(), "targets.json")
	result, err := service.Select(Request{
		DBPath:  dbPath,
		Mode:    "manual",
		File:    "src/app.go",
		OutPath: outPath,
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(result.Targets) < 2 {
		t.Fatalf("expected at least 2 targets, got %d", len(result.Targets))
	}
	if result.Targets[0].TargetNodeID != "go:repo:src/app.go:function:pkg.Handle" {
		t.Fatalf("expected entrypoint first, got %s", result.Targets[0].TargetNodeID)
	}
	if result.Targets[1].TargetNodeID != "go:repo:src/app.go:function:pkg.Helper" {
		t.Fatalf("expected public api second, got %s", result.Targets[1].TargetNodeID)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out file: %v", err)
	}
	var decoded []reviewgraph.ResolvedTarget
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode targets file: %v", err)
	}
	if len(decoded) != len(result.Targets) {
		t.Fatalf("expected persisted targets to match in-memory result")
	}
}
