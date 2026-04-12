package build_review_bundle_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/reviewbundle"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_review_bundle"
)

func TestWorkflowCreatesReviewBundleFromFixtureWorkspace(t *testing.T) {
	app := newTestApplication(t)

	result, err := app.BuildReviewBundle.Run(build_review_bundle.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "single_go_service"),
	})
	if err != nil {
		t.Fatalf("build review bundle: %v", err)
	}
	if result.BundleVersion != reviewbundle.BundleVersionV1 {
		t.Fatalf("expected bundle version %q, got %q", reviewbundle.BundleVersionV1, result.BundleVersion)
	}
	if result.FileCount != 9 {
		t.Fatalf("expected 9 bundle files, got %d", result.FileCount)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", result.Warnings)
	}

	requiredFiles := []string{
		"review_bundle.json",
		"workspace_manifest.json",
		"repository_manifests.json",
		"service_manifests.json",
		"scan_warnings.json",
		"build_snapshot_result.json",
		"quality_report.json",
		"graph_nodes.jsonl",
		"graph_edges.jsonl",
	}
	for _, name := range requiredFiles {
		if _, err := os.Stat(filepath.Join(result.BundleDir, name)); err != nil {
			t.Fatalf("required bundle file %s missing: %v", name, err)
		}
	}

	bundle := loadBundle(t, result.ReviewBundlePath)
	if bundle.WorkspaceID != result.WorkspaceID {
		t.Fatalf("workspace id mismatch: %s != %s", bundle.WorkspaceID, result.WorkspaceID)
	}
	if bundle.SnapshotID != result.SnapshotID {
		t.Fatalf("snapshot id mismatch: %s != %s", bundle.SnapshotID, result.SnapshotID)
	}
	if bundle.Snapshot.ID != result.SnapshotID {
		t.Fatalf("snapshot metadata mismatch: %s != %s", bundle.Snapshot.ID, result.SnapshotID)
	}
	if len(bundle.Files) != result.FileCount {
		t.Fatalf("expected %d file entries, got %d", result.FileCount, len(bundle.Files))
	}
	if bundle.WorkspaceManifest.Warnings == nil {
		t.Fatal("expected workspace warnings to be a JSON array")
	}
}

func TestWorkflowHonorsOutDirOverride(t *testing.T) {
	app := newTestApplication(t)
	outDir := filepath.Join(t.TempDir(), "custom", "bundle-dir")

	result, err := app.BuildReviewBundle.Run(build_review_bundle.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "single_go_service"),
		OutDir:        outDir,
	})
	if err != nil {
		t.Fatalf("build review bundle with out dir: %v", err)
	}
	if result.BundleDir != filepath.Clean(outDir) {
		t.Fatalf("expected bundle dir %s, got %s", filepath.Clean(outDir), result.BundleDir)
	}
}

func TestReviewBundleSchemaListsRequiredFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "schemas", "review_bundle.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	expected := []string{
		"workspace_id",
		"snapshot_id",
		"bundle_version",
		"generated_at",
		"workspace_manifest",
		"repository_manifests",
		"service_manifests",
		"snapshot",
		"quality_report",
		"graph",
		"files",
	}
	for _, name := range expected {
		if !slices.Contains(schema.Required, name) {
			t.Fatalf("schema missing required field %q", name)
		}
	}
}

func newTestApplication(t *testing.T) *bootstrap.Application {
	t.Helper()
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return app
}

func loadBundle(t *testing.T, path string) reviewbundle.Bundle {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read review bundle: %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("unmarshal review bundle: %v", err)
	}
	return bundle
}
