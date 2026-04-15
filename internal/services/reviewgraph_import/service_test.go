package reviewgraph_import_test

import (
	"encoding/json"
	"path/filepath"
	"os"
	"strings"
	"testing"

	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	legacygraph "analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/review_graph_import"
)

func TestReviewGraphImportDropsLegacyTests(t *testing.T) {
	app := newReviewGraphTestApplication(t)
	snapshot, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "relation_quality_app"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	result, err := app.ReviewGraphImport.Run(review_graph_import.Request{
		WorkspaceID: snapshot.WorkspaceID,
		SnapshotID:  snapshot.Snapshot.ID,
	})
	if err != nil {
		t.Fatalf("review graph import: %v", err)
	}
	if result.Manifest.Counts.DroppedTestNodeCount == 0 {
		t.Fatal("expected dropped test nodes to be reported")
	}
	store, err := reviewsqlite.New(result.DBPath)
	if err != nil {
		t.Fatalf("open review db: %v", err)
	}
	defer store.Close()
	nodes, err := store.ListNodes()
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	for _, node := range nodes {
		if strings.Contains(node.FilePath, "test_app.py") || strings.Contains(node.FilePath, ".test.ts") {
			t.Fatalf("expected test-related node to be excluded, got %+v", node)
		}
	}
}

func TestReviewGraphImportCreatesTopicNodesFromConfig(t *testing.T) {
	app := newReviewGraphTestApplication(t)
	snapshot, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "boundary_hints"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	result, err := app.ReviewGraphImport.Run(review_graph_import.Request{
		WorkspaceID: snapshot.WorkspaceID,
		SnapshotID:  snapshot.Snapshot.ID,
	})
	if err != nil {
		t.Fatalf("review graph import: %v", err)
	}
	if filepath.Base(result.DBPath) != "review_graph.sqlite" {
		t.Fatalf("expected default review graph db name, got %s", result.DBPath)
	}
	if result.Manifest.AsyncVersion != reviewgraph.AsyncV2Version {
		t.Fatalf("expected async version %q, got %q", reviewgraph.AsyncV2Version, result.Manifest.AsyncVersion)
	}
	store, err := reviewsqlite.New(result.DBPath)
	if err != nil {
		t.Fatalf("open review db: %v", err)
	}
	defer store.Close()
	nodes, err := store.ListNodes()
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	found := false
	for _, node := range nodes {
		if node.Kind == reviewgraph.NodeEventTopic && node.Symbol == "analysis.events" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected Kafka topic bridge node imported from config")
	}
}

func TestReviewGraphImportPreservesBoundaryCalls(t *testing.T) {
	app := newReviewGraphTestApplication(t)
	tempDir := t.TempDir()
	nodesPath := filepath.Join(tempDir, "graph_nodes.jsonl")
	edgesPath := filepath.Join(tempDir, "graph_edges.jsonl")
	repoManifestPath := filepath.Join(tempDir, "repository_manifests.json")
	serviceManifestPath := filepath.Join(tempDir, "service_manifests.json")
	qualityReportPath := filepath.Join(tempDir, "quality_report.json")
	outDBPath := filepath.Join(tempDir, "review_graph.sqlite")

	snapshotID := "snap_demo"
	nodes := []legacygraph.Node{
		{
			ID:            "node:api.handle",
			Kind:          legacygraph.NodeEndpoint,
			CanonicalName: "api.Handle",
			Language:      "go",
			RepositoryID:   "repo-api",
			FilePath:      "api/api.go",
			SnapshotID:    snapshotID,
			Properties:    map[string]string{"kind": "route_handler", "name": "Handle"},
		},
		{
			ID:            "node:worker.process",
			Kind:          legacygraph.NodeSymbol,
			CanonicalName: "worker.Process",
			Language:      "python",
			RepositoryID:   "repo-worker",
			FilePath:      "worker/handler.py",
			SnapshotID:    snapshotID,
			Properties:    map[string]string{"kind": "function", "name": "Process"},
		},
	}
	edges := []legacygraph.Edge{
		{
			ID:         "edge:api->worker",
			Kind:       legacygraph.EdgeCallsHTTP,
			From:       "node:api.handle",
			To:         "node:worker.process",
			Evidence:   legacygraph.Evidence{Type: "call", Source: "test", Details: "api -> worker"},
			Confidence: legacygraph.Confidence{Tier: legacygraph.ConfidenceConfirmed, Score: 1.0},
			SnapshotID: snapshotID,
		},
	}
	if err := writeJSONL(nodesPath, nodes); err != nil {
		t.Fatalf("write nodes: %v", err)
	}
	if err := writeJSONL(edgesPath, edges); err != nil {
		t.Fatalf("write edges: %v", err)
	}
	repoManifests := []map[string]any{
		{
			"id":            "repo-api",
			"name":          "api",
			"root_path":     "repo-api",
			"role":          "service",
			"tech_stack":    map[string]any{"languages": []string{"go"}},
			"go_files":      []string{"api/api.go"},
			"config_files":  []string{},
			"candidate_services": []map[string]any{},
		},
		{
			"id":            "repo-worker",
			"name":          "worker",
			"root_path":     "repo-worker",
			"role":          "service",
			"tech_stack":    map[string]any{"languages": []string{"python"}},
			"go_files":      []string{},
			"python_files":  []string{"worker/handler.py"},
			"config_files":  []string{},
			"candidate_services": []map[string]any{},
		},
	}
	serviceManifests := []map[string]any{
		{
			"id":           "svc-api",
			"name":         "api",
			"repository_id": "repo-api",
			"root_path":    "repo-api/api",
			"entrypoints":  []string{"api/api.go"},
			"boundaries":   []string{"http"},
		},
		{
			"id":           "svc-worker",
			"name":         "worker",
			"repository_id": "repo-worker",
			"root_path":    "repo-worker/worker",
			"entrypoints":  []string{"worker/handler.py"},
			"boundaries":   []string{"kafka"},
		},
	}
	qualityReport := map[string]any{
		"snapshot_id": snapshotID,
		"issue_counts": map[string]any{},
		"gaps":       []any{},
	}
	writeJSON := func(path string, payload any) {
		t.Helper()
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", path, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	writeJSON(repoManifestPath, repoManifests)
	writeJSON(serviceManifestPath, serviceManifests)
	writeJSON(qualityReportPath, qualityReport)

	result, err := app.ReviewGraphImport.Run(review_graph_import.Request{
		WorkspaceID:         "ws_demo",
		SnapshotID:          snapshotID,
		NodesPath:           nodesPath,
		EdgesPath:           edgesPath,
		RepoManifestPath:    repoManifestPath,
		ServiceManifestPath: serviceManifestPath,
		QualityReportPath:   qualityReportPath,
		OutDBPath:           outDBPath,
	})
	if err != nil {
		t.Fatalf("review graph import: %v", err)
	}
	store, err := reviewsqlite.New(result.DBPath)
	if err != nil {
		t.Fatalf("open review db: %v", err)
	}
	defer store.Close()
	edgesOut, err := store.ListEdges()
	if err != nil {
		t.Fatalf("list edges: %v", err)
	}
	if len(edgesOut) != 1 {
		t.Fatalf("expected preserved boundary edge, got %d: %#v", len(edgesOut), edgesOut)
	}
	if edgesOut[0].EdgeType != reviewgraph.EdgeCalls {
		t.Fatalf("expected boundary edge to import as CALLS, got %s", edgesOut[0].EdgeType)
	}
	if !strings.Contains(edgesOut[0].MetadataJSON, "CALLS_HTTP") {
		t.Fatalf("expected legacy boundary kind in metadata, got %s", edgesOut[0].MetadataJSON)
	}
}

func TestReviewGraphImportDropsGeneratedProtoRuntime(t *testing.T) {
	app := newReviewGraphTestApplication(t)
	tempDir := t.TempDir()
	nodesPath := filepath.Join(tempDir, "graph_nodes.jsonl")
	edgesPath := filepath.Join(tempDir, "graph_edges.jsonl")
	repoManifestPath := filepath.Join(tempDir, "repository_manifests.json")
	serviceManifestPath := filepath.Join(tempDir, "service_manifests.json")
	qualityReportPath := filepath.Join(tempDir, "quality_report.json")
	outDBPath := filepath.Join(tempDir, "review_graph.sqlite")

	snapshotID := "snap_proto"
	nodes := []legacygraph.Node{
		{
			ID:            "node:generated.proto.runtime",
			Kind:          legacygraph.NodeSymbol,
			CanonicalName: "proto.ScanServiceClient.Predict",
			Language:      "go",
			RepositoryID:  "repo-api",
			FilePath:      "pkg/proto/scan_service_grpc.pb.go",
			SnapshotID:    snapshotID,
			Properties:    map[string]string{"kind": "function", "name": "Predict"},
		},
		{
			ID:            "node:real.handler",
			Kind:          legacygraph.NodeSymbol,
			CanonicalName: "api.Handle",
			Language:      "go",
			RepositoryID:  "repo-api",
			FilePath:      "cmd/api.go",
			SnapshotID:    snapshotID,
			Properties:    map[string]string{"kind": "function", "name": "Handle"},
		},
	}
	edges := []legacygraph.Edge{
		{
			ID:         "edge:generated->real",
			Kind:       legacygraph.EdgeCalls,
			From:       "node:generated.proto.runtime",
			To:         "node:real.handler",
			Evidence:   legacygraph.Evidence{Type: "call", Source: "test", Details: "generated proto runtime"},
			Confidence: legacygraph.Confidence{Tier: legacygraph.ConfidenceConfirmed, Score: 1.0},
			SnapshotID: snapshotID,
		},
	}
	if err := writeJSONL(nodesPath, nodes); err != nil {
		t.Fatalf("write nodes: %v", err)
	}
	if err := writeJSONL(edgesPath, edges); err != nil {
		t.Fatalf("write edges: %v", err)
	}

	repoManifests := []map[string]any{
		{
			"id":                 "repo-api",
			"name":               "api",
			"root_path":          "repo-api",
			"role":               "service",
			"tech_stack":         map[string]any{"languages": []string{"go"}},
			"go_files":           []string{"cmd/api.go", "pkg/proto/scan_service_grpc.pb.go"},
			"config_files":       []string{},
			"candidate_services": []map[string]any{},
		},
	}
	serviceManifests := []map[string]any{
		{
			"id":            "svc-api",
			"name":          "api",
			"repository_id": "repo-api",
			"root_path":     "repo-api/cmd",
			"entrypoints":   []string{"cmd/api.go"},
			"boundaries":    []string{"http"},
		},
	}
	qualityReport := map[string]any{
		"snapshot_id":  snapshotID,
		"issue_counts": map[string]any{},
		"gaps":         []any{},
	}

	writeJSON := func(path string, payload any) {
		t.Helper()
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", path, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	writeJSON(repoManifestPath, repoManifests)
	writeJSON(serviceManifestPath, serviceManifests)
	writeJSON(qualityReportPath, qualityReport)

	result, err := app.ReviewGraphImport.Run(review_graph_import.Request{
		WorkspaceID:         "ws_proto",
		SnapshotID:          snapshotID,
		NodesPath:           nodesPath,
		EdgesPath:           edgesPath,
		RepoManifestPath:    repoManifestPath,
		ServiceManifestPath: serviceManifestPath,
		QualityReportPath:   qualityReportPath,
		OutDBPath:           outDBPath,
	})
	if err != nil {
		t.Fatalf("review graph import: %v", err)
	}
	if result.Manifest.Counts.DroppedGeneratedNodes == 0 {
		t.Fatalf("expected generated proto runtime nodes to be counted as dropped, got %+v", result.Manifest.Counts)
	}

	store, err := reviewsqlite.New(result.DBPath)
	if err != nil {
		t.Fatalf("open review db: %v", err)
	}
	defer store.Close()
	nodesOut, err := store.ListNodes()
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	for _, node := range nodesOut {
		if strings.Contains(node.FilePath, "pkg/proto/") || strings.HasSuffix(node.FilePath, ".pb.go") {
			t.Fatalf("expected generated protobuf runtime node to be excluded, got %+v", node)
		}
	}
}

func writeJSONL[T any](path string, payload []T) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, item := range payload {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func newReviewGraphTestApplication(t *testing.T) *bootstrap.Application {
	t.Helper()
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
	cfg.ProgressMode = "quiet"
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return app
}
