package integration

import (
	"os"
	"path/filepath"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
)

func TestEndToEndWorkflows(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = cfg.ArtifactRoot + "/analysis.sqlite"
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "single_go_service"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snapshotResult.Snapshot.ID == "" {
		t.Fatal("expected snapshot id")
	}
	blast, err := app.BlastRadius.Run(blast_radius.Request{
		WorkspaceID: snapshotResult.WorkspaceID,
		SnapshotID:  snapshotResult.Snapshot.ID,
		Target:      "service.Handle",
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("blast radius: %v", err)
	}
	if len(blast.QueryResult.Impacted) == 0 {
		t.Fatal("expected impacted nodes")
	}
	tests, err := app.ImpactedTests.Run(impacted_tests.Request{
		WorkspaceID: snapshotResult.WorkspaceID,
		SnapshotID:  snapshotResult.Snapshot.ID,
		Target:      "service.Handle",
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("impacted tests: %v", err)
	}
	if len(tests.QueryResult.Tests) == 0 {
		t.Fatal("expected impacted tests")
	}
}

func TestBuildSnapshotUsesWorkspaceScopedSQLiteByDefault(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "single_go_service"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	workspaceSQLite := filepath.Join(cfg.ArtifactRoot, "workspaces", snapshotResult.WorkspaceID, "analysis.sqlite")
	if _, err := os.Stat(workspaceSQLite); err != nil {
		t.Fatalf("expected workspace sqlite at %s: %v", workspaceSQLite, err)
	}
	rootSQLite := filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
	if _, err := os.Stat(rootSQLite); err == nil {
		t.Fatalf("did not expect root sqlite at %s", rootSQLite)
	}
}

func TestBuildSnapshotIndexesMixedLanguageWorkspace(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "mixed_language_app"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	foundPython := false
	foundJSOrTS := false
	foundSearchable := false
	for _, node := range snapshotResult.Snapshot.Nodes {
		if node.Language == "python" {
			foundPython = true
		}
		if node.Language == "javascript" || node.Language == "typescript" {
			foundJSOrTS = true
		}
		if (node.Kind == graph.NodeSymbol || node.Kind == graph.NodeTest) && node.CanonicalName == "src.app.handle_request" {
			foundSearchable = true
		}
	}
	if !foundPython {
		t.Fatal("expected python nodes in mixed-language snapshot")
	}
	if !foundJSOrTS {
		t.Fatal("expected javascript/typescript nodes in mixed-language snapshot")
	}
	if !foundSearchable {
		t.Fatal("expected python symbol src.app.handle_request to be indexed")
	}
}
