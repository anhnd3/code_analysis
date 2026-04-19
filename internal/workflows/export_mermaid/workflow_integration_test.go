package export_mermaid_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
)

func TestWorkflowFailsOnEmptyRootsAndWritesDebugBundle(t *testing.T) {
	app := newWorkflowTestApplication(t)
	debugDir := t.TempDir()

	_, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    "ws_test",
		SnapshotID:     "snap_test",
		RootType:       export_mermaid.RootFilterHTTP,
		DebugBundleDir: debugDir,
	}, repository.Inventory{}, graph.GraphSnapshot{})
	if err == nil {
		t.Fatal("expected export to fail when no rooted flow exists")
	}
	if !strings.Contains(err.Error(), "no http roots remained") {
		t.Fatalf("expected explicit empty-root failure, got %v", err)
	}

	for _, name := range []string{"boundary_roots.json", "boundary_diagnostics.json", "resolved_roots.json"} {
		if _, statErr := os.Stat(filepath.Join(debugDir, name)); statErr != nil {
			t.Fatalf("expected debug artifact %s on empty-root failure: %v", name, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(debugDir, "flow_bundle.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected flow_bundle.json to be absent before flow stitching, got %v", statErr)
	}
}

func TestWorkflowProducesNonEmptyArtifactsForAccessorFixture(t *testing.T) {
	app := newWorkflowTestApplication(t)
	workspace := fixtures.WorkspacePath(t, "gin_accessor_hardening")

	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	debugDir := t.TempDir()
	result, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    snapshotResult.WorkspaceID,
		SnapshotID:     snapshotResult.Snapshot.ID,
		RootType:       export_mermaid.RootFilterHTTP,
		DebugBundleDir: debugDir,
	}, snapshotResult.Inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid: %v", err)
	}
	if result.MermaidCode != "" {
		t.Fatalf("expected multi-root export to leave MermaidCode empty, got %q", result.MermaidCode)
	}
	if len(result.RootExports) <= 1 {
		t.Fatalf("expected multi-root per-root exports, got %+v", result.RootExports)
	}

	for _, name := range []string{"boundary_roots.json", "boundary_diagnostics.json", "resolved_roots.json", "flow_bundle.json", "boundary_bundle.json", "root_exports.json", "semantic_audit.json"} {
		if _, statErr := os.Stat(filepath.Join(debugDir, name)); statErr != nil {
			t.Fatalf("expected debug artifact %s after successful export: %v", name, statErr)
		}
	}
	for _, unexpected := range []string{"reduced_chain.json", "sequence_model.json", "diagram.mmd"} {
		if _, statErr := os.Stat(filepath.Join(debugDir, unexpected)); !os.IsNotExist(statErr) {
			t.Fatalf("expected multi-root export to skip %s at debug root, got %v", unexpected, statErr)
		}
	}

	rootExports := readRootExportsFile(t, filepath.Join(debugDir, "root_exports.json"))
	if len(rootExports) != len(result.RootExports) {
		t.Fatalf("expected debug root export manifest to mirror result, got %d vs %d", len(rootExports), len(result.RootExports))
	}
	rendered := 0
	for _, rootExport := range rootExports {
		if rootExport.Status == export_mermaid.RootExportSkipped {
			if rootExport.Reason == "" {
				t.Fatalf("expected skip reason for %+v", rootExport)
			}
			continue
		}
		if rootExport.Status != export_mermaid.RootExportRendered {
			t.Fatalf("expected rendered or skipped accessor root export, got %+v", rootExport)
		}
		rendered++
		for _, name := range []string{"reduced_chain.json", "review_flow.json", "review_flow_build.json", "sequence_model.json", "diagram.mmd", "semantic_audit.json"} {
			if _, statErr := os.Stat(filepath.Join(debugDir, "roots", rootExport.Slug, name)); statErr != nil {
				t.Fatalf("expected per-root debug artifact %s for %s: %v", name, rootExport.Slug, statErr)
			}
		}
	}
	if rendered == 0 {
		t.Fatal("expected at least one rendered root export")
	}
}

func TestWorkflowPreservesSingleRootSelectorBehavior(t *testing.T) {
	app := newWorkflowTestApplication(t)
	workspace := fixtures.WorkspacePath(t, "gin_accessor_hardening")

	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	debugDir := t.TempDir()
	result, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    snapshotResult.WorkspaceID,
		SnapshotID:     snapshotResult.Snapshot.ID,
		RootType:       export_mermaid.RootFilterHTTP,
		RootSelector:   "GET /health",
		DebugBundleDir: debugDir,
	}, snapshotResult.Inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid with selector: %v", err)
	}
	if result.MermaidCode == "" || result.MermaidCode == "sequenceDiagram\n" {
		t.Fatalf("expected selector-narrowed single-root Mermaid output, got %q", result.MermaidCode)
	}
	if len(result.RootExports) != 1 || result.RootExports[0].CanonicalName != "GET /health" {
		t.Fatalf("expected one selected root export, got %+v", result.RootExports)
	}
	for _, name := range []string{"reduced_chain.json", "review_flow.json", "review_flow_build.json", "sequence_model.json", "diagram.mmd", "root_exports.json", "semantic_audit.json"} {
		if _, statErr := os.Stat(filepath.Join(debugDir, name)); statErr != nil {
			t.Fatalf("expected single-root debug artifact %s: %v", name, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(debugDir, "roots")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no per-root debug directory in single-root mode, got %v", statErr)
	}
}

func TestWorkflowSupportsExplicitReducedDebugMode(t *testing.T) {
	app := newWorkflowTestApplication(t)
	workspace := fixtures.WorkspacePath(t, "gin_accessor_hardening")

	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	debugDir := t.TempDir()
	_, err = app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    snapshotResult.WorkspaceID,
		SnapshotID:     snapshotResult.Snapshot.ID,
		RootType:       export_mermaid.RootFilterHTTP,
		RootSelector:   "GET /health",
		RenderMode:     export_mermaid.RenderModeReducedDebug,
		DebugBundleDir: debugDir,
	}, snapshotResult.Inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid in reduced_debug mode: %v", err)
	}

	for _, name := range []string{"reduced_chain.json", "sequence_model.json", "diagram.mmd", "root_exports.json", "semantic_audit.json"} {
		if _, statErr := os.Stat(filepath.Join(debugDir, name)); statErr != nil {
			t.Fatalf("expected reduced_debug artifact %s: %v", name, statErr)
		}
	}
	for _, unexpected := range []string{"review_flow.json", "review_flow_build.json"} {
		if _, statErr := os.Stat(filepath.Join(debugDir, unexpected)); !os.IsNotExist(statErr) {
			t.Fatalf("expected reduced_debug mode to skip %s, got %v", unexpected, statErr)
		}
	}
}

func TestWorkflowIncludeCandidatesEmitsExtraCandidateArtifacts(t *testing.T) {
	app := newWorkflowTestApplication(t)
	workspace := fixtures.WorkspacePath(t, "gin_accessor_hardening")

	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	result, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:       snapshotResult.WorkspaceID,
		SnapshotID:        snapshotResult.Snapshot.ID,
		RootType:          export_mermaid.RootFilterHTTP,
		RootSelector:      "GET /health",
		IncludeCandidates: true,
	}, snapshotResult.Inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid with IncludeCandidates: %v", err)
	}

	foundCandidateSequence := false
	foundCandidateDiagram := false
	for _, ref := range result.RootExports[0].ArtifactRefs {
		switch filepath.Base(ref.Path) {
		case "candidate_sequence_model__faithful.json", "candidate_sequence_model__async_summarized.json":
			foundCandidateSequence = true
		case "candidate_diagram__faithful.mmd", "candidate_diagram__async_summarized.mmd":
			foundCandidateDiagram = true
		}
	}
	if !foundCandidateSequence || !foundCandidateDiagram {
		t.Fatalf("expected candidate render artifacts in root export refs, got %+v", result.RootExports[0].ArtifactRefs)
	}
}

func newWorkflowTestApplication(t *testing.T) *bootstrap.Application {
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

func readRootExportsFile(t *testing.T, path string) []export_mermaid.RootExport {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var exports []export_mermaid.RootExport
	if err := json.Unmarshal(data, &exports); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return exports
}
