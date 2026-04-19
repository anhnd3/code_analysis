package tests

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
)

// We check both the flag (if defined in this package) and the env var.
var updateGoldens = flag.Bool("update-goldens", false, "update golden files")

func TestExportMermaidGin(t *testing.T) {
	fixturesList := []string{
		"gin_route_direct",
		"gin_route_closure",
		"gin_route_async",
		"gin_processor_branch",
		"nethttp_route_direct",
		"nethttp_route_factory",
		"grpc_gateway_registration",
		"zpa_camera_config_be",
	}

	for _, fixtureName := range fixturesList {
		t.Run(fixtureName, func(t *testing.T) {
			runGinExportTest(t, fixtureName)
		})
	}
}

func runGinExportTest(t *testing.T, fixtureName string) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	workspacePath := fixtures.WorkspacePath(t, fixtureName)
	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspacePath,
	})
	if err != nil {
		t.Fatalf("build snapshot for %s: %v", fixtureName, err)
	}

	req := export_mermaid.Request{
		WorkspaceID: snapshotResult.WorkspaceID,
		SnapshotID:  snapshotResult.Snapshot.ID,
		RootType:    export_mermaid.RootFilterHTTP,
	}

	inventory := snapshotResult.Inventory

	result, err := app.ExportMermaid.Run(req, inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid for %s: %v", fixtureName, err)
	}

	// Persist golden artifacts
	// We use a testdata directory relative to this test file
	goldenBase := filepath.Join("testdata", "golden", "gin_export", fixtureName)

	for _, ref := range result.ArtifactRefs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatalf("read artifact %s: %v", ref.Path, err)
		}

		goldenPath := filepath.Join(goldenBase, filepath.Base(ref.Path))
		assertGoldenContent(t, goldenPath, data)
	}
}

func assertGoldenContent(t *testing.T, path string, actual []byte) {
	t.Helper()
	update := *updateGoldens || os.Getenv("UPDATE_GOLDEN") == "1"

	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		if update {
			return // already written
		}
		t.Fatalf("read golden file %s: %v (run with UPDATE_GOLDEN=1 to initialize)", path, err)
	}

	if !slices.Equal(actual, expected) {
		t.Errorf("golden mismatch for %s", path)
	}
}
