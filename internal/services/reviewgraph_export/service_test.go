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

func TestExportWritesOverallWorkflowDocs(t *testing.T) {
	dbPath, targetsPath := seedExportGraph(t, true)
	targets := []reviewgraph.ResolvedTarget{
		{TargetNodeID: "go:repo:svc.go:function:svc.Process", DisplayName: "svc.Process", Reason: "manual_symbol", SourceInput: "svc.Process"},
	}
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
	if len(result.FlowPaths) != 2 {
		t.Fatalf("expected two flow files, got %d", len(result.FlowPaths))
	}
	if result.ThreadsIndexPath != "" || len(result.ThreadOverviewPaths) != 0 || len(result.ThreadFocusPaths) != 0 {
		t.Fatalf("expected no companion outputs in overall mode, got threadsIndex=%q overviews=%d focus=%d", result.ThreadsIndexPath, len(result.ThreadOverviewPaths), len(result.ThreadFocusPaths))
	}
	if result.ResidualPath != "" || result.DiagnosticsPath != "" {
		t.Fatalf("expected no extra summary files in overall mode, got residual=%q diagnostics=%q", result.ResidualPath, result.DiagnosticsPath)
	}
	assertPathMissing(t, filepath.Join(result.ReviewDir, "flows"))
	assertPathMissing(t, filepath.Join(result.ReviewDir, "threads"))
	assertPathMissing(t, filepath.Join(result.ReviewDir, "summaries"))
	flows := map[string]string{}
	for _, path := range result.FlowPaths {
		flow, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read flow file %s: %v", path, err)
		}
		flows[filepath.Base(path)] = string(flow)
	}
	if syncFlow, ok := flows["01_sync_system.md"]; !ok || !strings.Contains(syncFlow, "# Synchronous System Flow") || !strings.Contains(syncFlow, "## Start Flow") || !strings.Contains(syncFlow, "api: Process") || !strings.Contains(syncFlow, "-- gRPC -->") || !strings.Contains(syncFlow, "scan: Predict") || strings.Contains(syncFlow, "```mermaid") {
		t.Fatalf("expected stitched sync workflow output, got:\n%v", flows)
	}
	if asyncFlow, ok := flows["02_async_system.md"]; !ok || !strings.Contains(asyncFlow, "# Asynchronous System Flow") || !strings.Contains(asyncFlow, "order.created") || !strings.Contains(asyncFlow, "worker: HandleOrderCreated") || strings.Contains(asyncFlow, "```mermaid") {
		t.Fatalf("expected stitched async workflow output, got:\n%v", flows)
	}
	index, err := os.ReadFile(result.IndexPath)
	if err != nil {
		t.Fatalf("read index file: %v", err)
	}
	if !strings.Contains(string(index), "starting points: 1") {
		t.Fatalf("expected index summary, got:\n%s", string(index))
	}
	if !strings.Contains(string(index), "01_sync_system.md") || !strings.Contains(string(index), "02_async_system.md") {
		t.Fatalf("expected stitched workflow file links in index, got:\n%s", string(index))
	}
	if !strings.Contains(string(index), "weak async match") || !strings.Contains(string(index), "Residual Coverage") {
		t.Fatalf("expected diagnostics and residual summary in index, got:\n%s", string(index))
	}
}

func TestExportWritesSyncOnlyWorkflowDoc(t *testing.T) {
	dbPath, targetsPath := seedExportGraph(t, false)
	targets := []reviewgraph.ResolvedTarget{
		{TargetNodeID: "go:repo:svc.go:function:svc.Process", DisplayName: "svc.Process", Reason: "manual_symbol", SourceInput: "svc.Process"},
	}
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
		t.Fatalf("expected one sync-only flow file, got %d", len(result.FlowPaths))
	}
	if result.ResidualPath != "" || result.DiagnosticsPath != "" {
		t.Fatalf("expected no extra summary files in overall mode, got residual=%q diagnostics=%q", result.ResidualPath, result.DiagnosticsPath)
	}
	flow, err := os.ReadFile(result.FlowPaths[0])
	if err != nil {
		t.Fatalf("read flow file: %v", err)
	}
	if filepath.Base(result.FlowPaths[0]) != "01_sync_system.md" {
		t.Fatalf("expected sync workflow file name, got %s", filepath.Base(result.FlowPaths[0]))
	}
	if strings.Contains(string(flow), "Async Workflow") || strings.Contains(string(flow), "Asynchronous Workflow") || strings.Contains(string(flow), "```mermaid") {
		t.Fatalf("expected no async workflow content in sync-only case, got:\n%s", string(flow))
	}
	index, err := os.ReadFile(result.IndexPath)
	if err != nil {
		t.Fatalf("read index file: %v", err)
	}
	if strings.Contains(string(index), "02_async_system.md") {
		t.Fatalf("expected no async workflow link in index, got:\n%s", string(index))
	}
	assertPathMissing(t, filepath.Join(result.ReviewDir, "flows"))
	assertPathMissing(t, filepath.Join(result.ReviewDir, "threads"))
	assertPathMissing(t, filepath.Join(result.ReviewDir, "summaries"))
}

func TestExportRawModePreservesFlatPaths(t *testing.T) {
	dbPath, targetsPath := seedExportGraph(t, true)
	targets := []reviewgraph.ResolvedTarget{
		{TargetNodeID: "go:repo:svc.go:function:svc.Process", DisplayName: "svc.Process", Reason: "manual_symbol", SourceInput: "svc.Process"},
	}
	data, _ := json.MarshalIndent(targets, "", "  ")
	if err := os.WriteFile(targetsPath, data, 0o644); err != nil {
		t.Fatalf("write targets: %v", err)
	}

	service := New(reviewgraph_paths.New(t.TempDir()), reviewgraph_traverse.New())
	result, err := service.Export(Request{
		DBPath:       dbPath,
		TargetsFile:  targetsPath,
		Mode:         string(reviewgraph.TraversalFullFlow),
		RenderMode:   "raw",
		IncludeAsync: true,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	flow, err := os.ReadFile(result.FlowPaths[0])
	if err != nil {
		t.Fatalf("read flow file: %v", err)
	}
	if !strings.Contains(string(flow), "go:repo:svc.go:function:svc.Process -> go:repo:svc.go:function:svc.Dispatch") {
		t.Fatalf("expected raw flat path output, got:\n%s", string(flow))
	}
}

func TestExportOverviewCompanionViewSkipsFocusFiles(t *testing.T) {
	dbPath, targetsPath := seedExportGraph(t, true)
	targets := []reviewgraph.ResolvedTarget{
		{TargetNodeID: "go:repo:svc.go:function:svc.Process", DisplayName: "svc.Process", Reason: "manual_symbol", SourceInput: "svc.Process"},
	}
	data, _ := json.MarshalIndent(targets, "", "  ")
	if err := os.WriteFile(targetsPath, data, 0o644); err != nil {
		t.Fatalf("write targets: %v", err)
	}

	service := New(reviewgraph_paths.New(t.TempDir()), reviewgraph_traverse.New())
	result, err := service.Export(Request{
		DBPath:        dbPath,
		TargetsFile:   targetsPath,
		Mode:          string(reviewgraph.TraversalFullFlow),
		CompanionView: "overview",
		IncludeAsync:  true,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if result.ThreadsIndexPath == "" || len(result.ThreadOverviewPaths) != 1 {
		t.Fatalf("expected overview companion outputs, got threadsIndex=%q overviews=%d", result.ThreadsIndexPath, len(result.ThreadOverviewPaths))
	}
	if len(result.ThreadFocusPaths) != 0 {
		t.Fatalf("expected no focus files in overview mode, got %d", len(result.ThreadFocusPaths))
	}
}

func seedExportGraph(t *testing.T, includeAsync bool) (string, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "review_graph.sqlite")
	store, err := reviewsqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	snapshotID := "snap_demo"
	nodes := []reviewgraph.Node{
		{ID: "go:repo:svc.go:function:svc.Process", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "svc.Process", FilePath: "svc.go", StartLine: 1, EndLine: 20, NodeRole: reviewgraph.RoleEntrypoint},
		{ID: "go:repo:svc.go:function:svc.Dispatch", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "svc.Dispatch", FilePath: "svc.go", StartLine: 22, EndLine: 40, NodeRole: reviewgraph.RoleNormal},
		{ID: "go:repo:scan_handler.go:grpc_method:scan.Predict", SnapshotID: snapshotID, Repo: "repo", Service: "scan", Language: "go", Kind: reviewgraph.NodeGRPCMethod, Symbol: "scan.Predict", FilePath: "scan_handler.go", StartLine: 1, EndLine: 20, NodeRole: reviewgraph.RoleBoundary},
		{ID: "go:repo:scan_service.go:function:scan.ServicePredict", SnapshotID: snapshotID, Repo: "repo", Service: "scan", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "scan.ServicePredict", FilePath: "scan_service.go", StartLine: 22, EndLine: 40, NodeRole: reviewgraph.RoleNormal},
		{ID: "go:repo:vector.go:function:vector.Search", SnapshotID: snapshotID, Repo: "repo", Service: "vector", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "vector.Search", FilePath: "vector.go", StartLine: 42, EndLine: 50, NodeRole: reviewgraph.RoleSharedInfra},
	}
	edges := []reviewgraph.Edge{
		{ID: "e1", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Process", DstID: "go:repo:svc.go:function:svc.Dispatch", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e2", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Dispatch", DstID: "go:repo:scan_handler.go:grpc_method:scan.Predict", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync, MetadataJSON: reviewsqlite.EncodeJSON(map[string]any{"legacy_kind": "CALLS_GRPC"})},
		{ID: "e3", SnapshotID: snapshotID, SrcID: "go:repo:scan_handler.go:grpc_method:scan.Predict", DstID: "go:repo:scan_service.go:function:scan.ServicePredict", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e4", SnapshotID: snapshotID, SrcID: "go:repo:scan_service.go:function:scan.ServicePredict", DstID: "go:repo:vector.go:function:vector.Search", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
	}
	if includeAsync {
		nodes = append(nodes,
			reviewgraph.Node{ID: "event_topic:kafka:order.created", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "yaml", Kind: reviewgraph.NodeEventTopic, Symbol: "order.created", FilePath: "configs/kafka.yaml", NodeRole: reviewgraph.RoleBoundary},
			reviewgraph.Node{ID: "go:repo:worker.go:function:worker.HandleOrderCreated", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "worker.HandleOrderCreated", FilePath: "worker.go", StartLine: 1, EndLine: 20, NodeRole: reviewgraph.RoleAsyncConsumer},
			reviewgraph.Node{ID: "go:repo:worker.go:method:worker.Handler.Commit", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeMethod, Symbol: "worker.Handler.Commit", FilePath: "worker.go", StartLine: 22, EndLine: 30, NodeRole: reviewgraph.RoleNormal},
		)
		edges = append(edges,
			reviewgraph.Edge{ID: "e5", SnapshotID: snapshotID, SrcID: "go:repo:scan_service.go:function:scan.ServicePredict", DstID: "event_topic:kafka:order.created", EdgeType: reviewgraph.EdgeEmitsEvent, FlowKind: reviewgraph.FlowAsync, Transport: "kafka", TopicOrChannel: "order.created"},
			reviewgraph.Edge{ID: "e6", SnapshotID: snapshotID, SrcID: "event_topic:kafka:order.created", DstID: "go:repo:worker.go:function:worker.HandleOrderCreated", EdgeType: reviewgraph.EdgeConsumesEvent, FlowKind: reviewgraph.FlowAsync, Transport: "kafka", TopicOrChannel: "order.created"},
			reviewgraph.Edge{ID: "e7", SnapshotID: snapshotID, SrcID: "go:repo:worker.go:function:worker.HandleOrderCreated", DstID: "go:repo:worker.go:method:worker.Handler.Commit", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		)
	}
	importManifest := reviewgraph.ImportManifest{
		WorkspaceID:     "ws_demo",
		SnapshotID:      snapshotID,
		ImporterVersion: reviewgraph.ImporterVersion,
		AsyncVersion:    reviewgraph.AsyncV2Version,
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
	return dbPath, filepath.Join(tempDir, "targets.json")
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", path, err)
	}
}
