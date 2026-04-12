package reviewgraph_export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/services/reviewgraph_paths"
	"analysis-module/internal/services/reviewgraph_traverse"
)

func TestExportWritesFlowIndexAndDiagnostics(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "review_graph.sqlite")
	store, err := reviewsqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	snapshotID := "snap_demo"
	nodes := []reviewgraph.Node{
		{ID: "go:repo:svc.go:function:svc.Process", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "svc.Process", FilePath: "svc.go", StartLine: 1, EndLine: 20, NodeRole: reviewgraph.RoleEntrypoint},
		{ID: "go:repo:svc.go:function:svc.Dispatch", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "svc.Dispatch", FilePath: "svc.go", StartLine: 22, EndLine: 40, NodeRole: reviewgraph.RoleNormal},
		{ID: "go:repo:svc.go:function:svc.Cycle", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "svc.Cycle", FilePath: "svc.go", StartLine: 42, EndLine: 50, NodeRole: reviewgraph.RoleNormal},
		{ID: "event_topic:kafka:order.created", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "yaml", Kind: reviewgraph.NodeEventTopic, Symbol: "order.created", FilePath: "configs/kafka.yaml", NodeRole: reviewgraph.RoleBoundary},
		{ID: "go:repo:worker.go:function:worker.HandleOrderCreated", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "worker.HandleOrderCreated", FilePath: "worker.go", StartLine: 1, EndLine: 20, NodeRole: reviewgraph.RoleAsyncConsumer},
		{ID: "go:repo:worker.go:function:worker.Commit", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "worker.Commit", FilePath: "worker.go", StartLine: 22, EndLine: 30, NodeRole: reviewgraph.RoleNormal},
	}
	edges := []reviewgraph.Edge{
		{ID: "e1", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Process", DstID: "go:repo:svc.go:function:svc.Dispatch", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e2", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Dispatch", DstID: "go:repo:svc.go:function:svc.Cycle", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e3", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Cycle", DstID: "go:repo:svc.go:function:svc.Dispatch", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e4", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Dispatch", DstID: "event_topic:kafka:order.created", EdgeType: reviewgraph.EdgeEmitsEvent, FlowKind: reviewgraph.FlowAsync, Transport: "kafka", TopicOrChannel: "order.created"},
		{ID: "e5", SnapshotID: snapshotID, SrcID: "event_topic:kafka:order.created", DstID: "go:repo:worker.go:function:worker.HandleOrderCreated", EdgeType: reviewgraph.EdgeConsumesEvent, FlowKind: reviewgraph.FlowAsync, Transport: "kafka", TopicOrChannel: "order.created"},
		{ID: "e6", SnapshotID: snapshotID, SrcID: "go:repo:worker.go:function:worker.HandleOrderCreated", DstID: "go:repo:worker.go:function:worker.Commit", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
	}
	importManifest := reviewgraph.ImportManifest{
		WorkspaceID:     "ws_demo",
		SnapshotID:      snapshotID,
		ImporterVersion: reviewgraph.ImporterVersion,
		AsyncVersion:    reviewgraph.AsyncHeuristicVersion,
		Diagnostics: []reviewgraph.ImportDiagnostic{
			{Category: "weak_async_match", Message: "weak async match", FilePath: "svc.go", Line: 7},
		},
	}
	artifacts := []reviewgraph.Artifact{
		{ID: reviewgraph.ArtifactID(reviewgraph.ArtifactImportManifest, "", ""), SnapshotID: snapshotID, ArtifactType: reviewgraph.ArtifactImportManifest, Path: "", MetadataJSON: reviewsqlite.EncodeJSON(importManifest)},
	}
	if err := store.ReplaceSnapshot(snapshotID, nodes, edges, artifacts); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}
	targets := []reviewgraph.ResolvedTarget{
		{TargetNodeID: "go:repo:svc.go:function:svc.Process", DisplayName: "svc.Process", Reason: "manual_symbol", SourceInput: "svc.Process"},
	}
	targetsPath := filepath.Join(t.TempDir(), "targets.json")
	data, _ := json.MarshalIndent(targets, "", "  ")
	if err := os.WriteFile(targetsPath, data, 0o644); err != nil {
		t.Fatalf("write targets: %v", err)
	}

	service := New(reviewgraph_paths.New(t.TempDir()), reviewgraph_traverse.New())
	result, err := service.Export(Request{
		DBPath:       dbPath,
		TargetsFile:  targetsPath,
		Mode:         string(reviewgraph.TraversalFullFlow),
		IncludeAsync: true,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(result.FlowPaths) != 1 {
		t.Fatalf("expected one flow file, got %d", len(result.FlowPaths))
	}
	flow, err := os.ReadFile(result.FlowPaths[0])
	if err != nil {
		t.Fatalf("read flow file: %v", err)
	}
	if !strings.Contains(string(flow), "Asynchronous Flow") || !strings.Contains(string(flow), "order.created") {
		t.Fatalf("expected async section in flow file, got:\n%s", string(flow))
	}
	if !strings.Contains(string(flow), "cycle") && !strings.Contains(string(flow), "possible loop risk") {
		t.Fatalf("expected cycle details in flow file, got:\n%s", string(flow))
	}
	index, err := os.ReadFile(result.IndexPath)
	if err != nil {
		t.Fatalf("read index file: %v", err)
	}
	if !strings.Contains(string(index), "starting points: 1") {
		t.Fatalf("expected index summary, got:\n%s", string(index))
	}
	diagnostics, err := os.ReadFile(result.DiagnosticsPath)
	if err != nil {
		t.Fatalf("read diagnostics file: %v", err)
	}
	if !strings.Contains(string(diagnostics), "weak async match") {
		t.Fatalf("expected diagnostics content, got:\n%s", string(diagnostics))
	}
}
