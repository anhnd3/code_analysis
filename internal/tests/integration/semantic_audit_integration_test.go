package integration

import (
	"path/filepath"
	"reflect"
	"testing"

	"analysis-module/internal/adapters/graphstore/sqlite"
	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
)

func TestSemanticAuditMatchesWorkspaceSQLiteSnapshot(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.ProgressMode = "quiet"

	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "nethttp_route_factory"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	firstDebugDir := t.TempDir()
	_, err = app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    snapshotResult.WorkspaceID,
		SnapshotID:     snapshotResult.Snapshot.ID,
		RootType:       export_mermaid.RootFilterHTTP,
		DebugBundleDir: firstDebugDir,
	}, snapshotResult.Inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid from in-memory snapshot: %v", err)
	}

	var inMemoryAudit flow_stitch.SemanticAudit
	readJSONFile(t, filepath.Join(firstDebugDir, "semantic_audit.json"), &inMemoryAudit)

	workspaceSQLite := filepath.Join(cfg.ArtifactRoot, "workspaces", snapshotResult.WorkspaceID, "analysis.sqlite")
	store, err := sqlite.New(workspaceSQLite)
	if err != nil {
		t.Fatalf("open workspace sqlite: %v", err)
	}
	loadedSnapshot, err := store.GetSnapshot(snapshotResult.Snapshot.ID)
	if err != nil {
		t.Fatalf("load snapshot from workspace sqlite: %v", err)
	}

	secondDebugDir := t.TempDir()
	_, err = app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    snapshotResult.WorkspaceID,
		SnapshotID:     snapshotResult.Snapshot.ID,
		RootType:       export_mermaid.RootFilterHTTP,
		DebugBundleDir: secondDebugDir,
	}, snapshotResult.Inventory, loadedSnapshot)
	if err != nil {
		t.Fatalf("export mermaid from sqlite-loaded snapshot: %v", err)
	}

	var sqliteAudit flow_stitch.SemanticAudit
	readJSONFile(t, filepath.Join(secondDebugDir, "semantic_audit.json"), &sqliteAudit)

	if !reflect.DeepEqual(inMemoryAudit, sqliteAudit) {
		t.Fatalf("expected semantic audit to match between in-memory and sqlite snapshots:\nin_memory=%+v\nsqlite=%+v", inMemoryAudit, sqliteAudit)
	}
	if len(inMemoryAudit.Roots) == 0 {
		t.Fatal("expected semantic audit roots")
	}
}
