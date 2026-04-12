package review_bundle_packager_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/app/progress"
	"analysis-module/internal/services/review_bundle_packager"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
)

func TestPackageWritesReviewBundleDirectory(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
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

	service := review_bundle_packager.New(cfg.ArtifactRoot, progress.NoopReporter{})
	result, err := service.Package(review_bundle_packager.Request{
		WorkspaceID:         snapshotResult.WorkspaceID,
		Snapshot:            snapshotResult.Snapshot,
		QualityReport:       snapshotResult.QualityReport,
		ArtifactRefs:        snapshotResult.ArtifactRefs,
		BuildSnapshotResult: snapshotResult,
	})
	if err != nil {
		t.Fatalf("package bundle: %v", err)
	}
	if result.FileCount != 9 {
		t.Fatalf("expected 9 bundle files, got %d", result.FileCount)
	}
	if !strings.HasSuffix(result.BundleDir, filepath.Join("snapshots", snapshotResult.Snapshot.ID, "review_bundle")) {
		t.Fatalf("unexpected bundle dir: %s", result.BundleDir)
	}
	if len(result.Bundle.ServiceManifests) != 1 {
		t.Fatalf("expected 1 service manifest, got %d", len(result.Bundle.ServiceManifests))
	}
	if result.Bundle.Graph.TotalNodeCount != len(result.Bundle.Graph.Nodes) {
		t.Fatalf("expected node count to match graph payload")
	}
	if _, err := os.Stat(result.ReviewBundlePath); err != nil {
		t.Fatalf("review bundle missing: %v", err)
	}
}

func TestPackageRejectsNonEmptyOutDir(t *testing.T) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
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

	outDir := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "keep.txt"), []byte("occupied"), 0o644); err != nil {
		t.Fatalf("seed out dir: %v", err)
	}

	service := review_bundle_packager.New(cfg.ArtifactRoot, progress.NoopReporter{})
	_, err = service.Package(review_bundle_packager.Request{
		WorkspaceID:         snapshotResult.WorkspaceID,
		Snapshot:            snapshotResult.Snapshot,
		QualityReport:       snapshotResult.QualityReport,
		ArtifactRefs:        snapshotResult.ArtifactRefs,
		BuildSnapshotResult: snapshotResult,
		OutDir:              outDir,
	})
	if err == nil {
		t.Fatal("expected non-empty out dir error")
	}
	if !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("expected empty-dir error, got %v", err)
	}
}
