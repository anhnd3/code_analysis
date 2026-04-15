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

func TestSelectDefaultWorkflowSkipsAsyncNoise(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "review_graph.sqlite")
	store, err := reviewsqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	snapshotID := "snap_demo"
	nodes := []reviewgraph.Node{
		{ID: "go:repo:src/app.go:function:pkg.Handle", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "pkg.Handle", FilePath: "src/app.go", StartLine: 1, EndLine: 10, NodeRole: reviewgraph.RoleEntrypoint},
		{ID: "go:repo:src/app.go:function:pkg.Public", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "pkg.Public", FilePath: "src/app.go", StartLine: 12, EndLine: 20, NodeRole: reviewgraph.RolePublicAPI},
		{ID: "go:repo:src/app.go:job:pkg.Tick", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeSchedulerJob, Symbol: "pkg.Tick", FilePath: "src/app.go", StartLine: 22, EndLine: 30, NodeRole: reviewgraph.RoleScheduler},
		{ID: "go:repo:src/app.go:function:pkg.Emit", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "pkg.Emit", FilePath: "src/app.go", StartLine: 32, EndLine: 40, NodeRole: reviewgraph.RoleAsyncProducer},
		{ID: "go:repo:src/app.go:function:pkg.Receive", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "pkg.Receive", FilePath: "src/app.go", StartLine: 42, EndLine: 50, NodeRole: reviewgraph.RoleAsyncConsumer},
	}
	if err := store.ReplaceSnapshot(snapshotID, nodes, nil, nil); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	service := New(reviewgraph_paths.New(t.TempDir()))
	result, err := service.Select(Request{
		DBPath: dbPath,
	})
	if err != nil {
		t.Fatalf("select default workflow: %v", err)
	}
	if len(result.Targets) != 2 {
		t.Fatalf("expected 2 primary workflow roots, got %d: %#v", len(result.Targets), result.Targets)
	}
	for _, target := range result.Targets {
		if target.TargetNodeID == "go:repo:src/app.go:function:pkg.Emit" || target.TargetNodeID == "go:repo:src/app.go:function:pkg.Receive" {
			t.Fatalf("did not expect async producer/consumer in default workflow targets: %#v", result.Targets)
		}
		if target.TargetNodeID == "go:repo:src/app.go:function:pkg.Public" {
			t.Fatalf("did not expect exported public helper in default workflow targets: %#v", result.Targets)
		}
	}
}

func TestSelectDefaultWorkflowPrefersBootstrapEntrypointsPerFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "review_graph.sqlite")
	store, err := reviewsqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	snapshotID := "snap_demo"
	nodes := []reviewgraph.Node{
		{ID: "go:repo:cmd/config.go:function:cmd.CurrentEnvironment", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "cmd.CurrentEnvironment", FilePath: "cmd/config.go", StartLine: 1, EndLine: 10, NodeRole: reviewgraph.RolePublicAPI},
		{ID: "go:repo:cmd/config.go:method:cmd.Environment.String", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeMethod, Symbol: "cmd.Environment.String", FilePath: "cmd/config.go", StartLine: 12, EndLine: 20, NodeRole: reviewgraph.RolePublicAPI},
		{ID: "go:repo:cmd/run.go:function:cmd.Execute", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "cmd.Execute", FilePath: "cmd/run.go", StartLine: 22, EndLine: 30, NodeRole: reviewgraph.RoleEntrypoint},
		{ID: "go:repo:cmd/server.go:function:cmd.NewService", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "cmd.NewService", FilePath: "cmd/server.go", StartLine: 32, EndLine: 40, NodeRole: reviewgraph.RoleEntrypoint},
		{ID: "go:repo:cmd/server.go:method:cmd.service.Close", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeMethod, Symbol: "cmd.service.Close", FilePath: "cmd/server.go", StartLine: 42, EndLine: 50, NodeRole: reviewgraph.RoleEntrypoint},
	}
	if err := store.ReplaceSnapshot(snapshotID, nodes, nil, nil); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	service := New(reviewgraph_paths.New(t.TempDir()))
	result, err := service.Select(Request{DBPath: dbPath})
	if err != nil {
		t.Fatalf("select default workflow: %v", err)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("expected one bootstrap workflow root, got %d: %#v", len(result.Targets), result.Targets)
	}
	if result.Targets[0].TargetNodeID != "go:repo:cmd/run.go:function:cmd.Execute" {
		t.Fatalf("expected cmd.Execute as workflow root, got %#v", result.Targets)
	}
}
