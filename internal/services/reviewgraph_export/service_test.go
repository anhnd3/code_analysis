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
	dbPath, targetsPath := seedExportGraph(t)
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
		t.Fatalf("expected one flow file, got %d", len(result.FlowPaths))
	}
	flow, err := os.ReadFile(result.FlowPaths[0])
	if err != nil {
		t.Fatalf("read flow file: %v", err)
	}
	if !strings.Contains(string(flow), "Asynchronous Flow") || !strings.Contains(string(flow), "order.created") {
		t.Fatalf("expected async section in flow file, got:\n%s", string(flow))
	}
	if !strings.Contains(string(flow), "Consumer Sync Context") || !strings.Contains(string(flow), "`svc.go`") || !strings.Contains(string(flow), "`Process`") {
		t.Fatalf("expected grouped sync and async tree rendering, got:\n%s", string(flow))
	}
	if strings.Contains(string(flow), "Derived Layer Summary") {
		t.Fatalf("did not expect obsolete layering summary, got:\n%s", string(flow))
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
	if !strings.Contains(string(index), "Companion Thread Views") || !strings.Contains(string(index), "threads/00_index.md") {
		t.Fatalf("expected threads companion link in index, got:\n%s", string(index))
	}
	if result.ThreadsIndexPath == "" || len(result.ThreadOverviewPaths) != 1 || len(result.ThreadFocusPaths) == 0 {
		t.Fatalf("expected companion outputs, got threadsIndex=%q overviews=%d focus=%d", result.ThreadsIndexPath, len(result.ThreadOverviewPaths), len(result.ThreadFocusPaths))
	}
	threadsIndex, err := os.ReadFile(result.ThreadsIndexPath)
	if err != nil {
		t.Fatalf("read threads index: %v", err)
	}
	if !strings.Contains(string(threadsIndex), "Thread Companion Views") || !strings.Contains(string(threadsIndex), "overview") {
		t.Fatalf("expected thread companion index content, got:\n%s", string(threadsIndex))
	}
	overview, err := os.ReadFile(result.ThreadOverviewPaths[0])
	if err != nil {
		t.Fatalf("read overview file: %v", err)
	}
	if !strings.Contains(string(overview), "## 1. Sync Thread") || !strings.Contains(string(overview), "## 2. Async Thread") || !strings.Contains(string(overview), "```mermaid") {
		t.Fatalf("expected mermaid overview content, got:\n%s", string(overview))
	}
	foundClassFocus := false
	for _, focusPath := range result.ThreadFocusPaths {
		focus, err := os.ReadFile(focusPath)
		if err != nil {
			t.Fatalf("read focus file %s: %v", focusPath, err)
		}
		if strings.Contains(string(focus), "Bucket Kind: `class`") {
			foundClassFocus = true
		}
	}
	if !foundClassFocus {
		t.Fatalf("expected at least one class-focused companion file, got: %v", result.ThreadFocusPaths)
	}
	diagnostics, err := os.ReadFile(result.DiagnosticsPath)
	if err != nil {
		t.Fatalf("read diagnostics file: %v", err)
	}
	if !strings.Contains(string(diagnostics), "weak async match") {
		t.Fatalf("expected diagnostics content, got:\n%s", string(diagnostics))
	}
}

func TestExportRawModePreservesFlatPaths(t *testing.T) {
	dbPath, targetsPath := seedExportGraph(t)
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
	dbPath, targetsPath := seedExportGraph(t)
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

func seedExportGraph(t *testing.T) (string, string) {
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
		{ID: "go:repo:svc.go:function:svc.Cycle", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "svc.Cycle", FilePath: "svc.go", StartLine: 42, EndLine: 50, NodeRole: reviewgraph.RoleNormal},
		{ID: "event_topic:kafka:order.created", SnapshotID: snapshotID, Repo: "repo", Service: "api", Language: "yaml", Kind: reviewgraph.NodeEventTopic, Symbol: "order.created", FilePath: "configs/kafka.yaml", NodeRole: reviewgraph.RoleBoundary},
		{ID: "go:repo:worker.go:function:worker.HandleOrderCreated", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeFunction, Symbol: "worker.HandleOrderCreated", FilePath: "worker.go", StartLine: 1, EndLine: 20, NodeRole: reviewgraph.RoleAsyncConsumer},
		{ID: "go:repo:worker.go:method:worker.Handler.Commit", SnapshotID: snapshotID, Repo: "repo", Service: "worker", Language: "go", Kind: reviewgraph.NodeMethod, Symbol: "worker.Handler.Commit", FilePath: "worker.go", StartLine: 22, EndLine: 30, NodeRole: reviewgraph.RoleNormal},
	}
	edges := []reviewgraph.Edge{
		{ID: "e1", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Process", DstID: "go:repo:svc.go:function:svc.Dispatch", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e2", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Dispatch", DstID: "go:repo:svc.go:function:svc.Cycle", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e3", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Cycle", DstID: "go:repo:svc.go:function:svc.Dispatch", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
		{ID: "e4", SnapshotID: snapshotID, SrcID: "go:repo:svc.go:function:svc.Dispatch", DstID: "event_topic:kafka:order.created", EdgeType: reviewgraph.EdgeEmitsEvent, FlowKind: reviewgraph.FlowAsync, Transport: "kafka", TopicOrChannel: "order.created"},
		{ID: "e5", SnapshotID: snapshotID, SrcID: "event_topic:kafka:order.created", DstID: "go:repo:worker.go:function:worker.HandleOrderCreated", EdgeType: reviewgraph.EdgeConsumesEvent, FlowKind: reviewgraph.FlowAsync, Transport: "kafka", TopicOrChannel: "order.created"},
		{ID: "e6", SnapshotID: snapshotID, SrcID: "go:repo:worker.go:function:worker.HandleOrderCreated", DstID: "go:repo:worker.go:method:worker.Handler.Commit", EdgeType: reviewgraph.EdgeCalls, FlowKind: reviewgraph.FlowSync},
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
