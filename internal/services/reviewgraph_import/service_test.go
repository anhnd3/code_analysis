package reviewgraph_import_test

import (
	"path/filepath"
	"strings"
	"testing"

	reviewsqlite "analysis-module/internal/adapters/reviewstore/sqlite"
	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
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
