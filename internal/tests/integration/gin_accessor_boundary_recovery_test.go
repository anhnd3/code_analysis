package integration

import (
	"path/filepath"
	"slices"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
)

func TestGinAccessorBoundaryRecoveryPersistsAndExports(t *testing.T) {
	app := newGraphTestApplication(t)
	workspace := fixtures.WorkspacePath(t, "gin_accessor_hardening")

	first := runGinAccessorRecovery(t, app, workspace)
	assertGinAccessorRecovery(t, first)

	waitForNextSnapshotIDTick()

	second := runGinAccessorRecovery(t, app, workspace)
	assertGinAccessorRecovery(t, second)

	if !slices.Equal(boundaryRootIDs(first.roots), boundaryRootIDs(second.roots)) {
		t.Fatalf("expected stable boundary root IDs across reruns:\nfirst=%v\nsecond=%v", boundaryRootIDs(first.roots), boundaryRootIDs(second.roots))
	}
	if !slices.Equal(endpointNodeSignatures(first.snapshot.Snapshot.Nodes), endpointNodeSignatures(second.snapshot.Snapshot.Nodes)) {
		t.Fatalf("expected stable endpoint node identity across reruns")
	}
	if !slices.Equal(boundaryEdgeSignatures(first.snapshot.Snapshot.Edges), boundaryEdgeSignatures(second.snapshot.Snapshot.Edges)) {
		t.Fatalf("expected stable REGISTERS_BOUNDARY edges across reruns")
	}
	if !slices.Equal(rootExportSlugs(first.exports), rootExportSlugs(second.exports)) {
		t.Fatalf("expected stable root export slugs across reruns:\nfirst=%v\nsecond=%v", rootExportSlugs(first.exports), rootExportSlugs(second.exports))
	}
}

type ginAccessorRun struct {
	snapshot build_snapshot.Result
	roots    []boundaryroot.Root
	resolved entrypoint.Result
	flow     flow.Bundle
	exports  []export_mermaid.RootExport
}

func runGinAccessorRecovery(t *testing.T, app *bootstrap.Application, workspace string) ginAccessorRun {
	t.Helper()

	buildResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	debugDir := t.TempDir()
	_, err = app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:    buildResult.WorkspaceID,
		SnapshotID:     buildResult.Snapshot.ID,
		RootType:       export_mermaid.RootFilterHTTP,
		DebugBundleDir: debugDir,
	}, buildResult.Inventory, buildResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid: %v", err)
	}

	var roots []boundaryroot.Root
	readJSONFile(t, filepath.Join(debugDir, "boundary_roots.json"), &roots)

	var resolved entrypoint.Result
	readJSONFile(t, filepath.Join(debugDir, "resolved_roots.json"), &resolved)

	var flowBundle flow.Bundle
	readJSONFile(t, filepath.Join(debugDir, "flow_bundle.json"), &flowBundle)

	var rootExports []export_mermaid.RootExport
	readJSONFile(t, filepath.Join(debugDir, "root_exports.json"), &rootExports)

	return ginAccessorRun{
		snapshot: buildResult,
		roots:    roots,
		resolved: resolved,
		flow:     flowBundle,
		exports:  rootExports,
	}
}

func assertGinAccessorRecovery(t *testing.T, run ginAccessorRun) {
	t.Helper()

	requiredCanonicals := []string{
		"GET /health",
		"GET /metrics/prom",
		"GET /v1/camera/config/all",
		"POST /v1/camera/detect-qr",
		"POST /v2/camera/detect-qr",
	}
	for _, canonical := range requiredCanonicals {
		root := rootByCanonicalNameOrFail(t, run.roots, canonical)
		if root.ID != boundaryroot.StableID(root) {
			t.Fatalf("expected stable root ID for %s, got %q", canonical, root.ID)
		}
	}

	if len(run.resolved.Roots) == 0 {
		t.Fatal("expected non-empty resolved roots")
	}
	if len(run.flow.Chains) == 0 {
		t.Fatal("expected non-empty stitched flow chains")
	}
	if len(run.exports) != len(run.resolved.Roots) {
		t.Fatalf("expected one root export per resolved root, got %d exports for %d roots", len(run.exports), len(run.resolved.Roots))
	}
	for _, rootExport := range run.exports {
		if rootExport.Status != export_mermaid.RootExportRendered {
			t.Fatalf("expected rendered root export for accessor fixture, got %+v", rootExport)
		}
		if len(rootExport.ArtifactRefs) != 3 {
			t.Fatalf("expected reduced/sequence/mermaid refs for %s, got %+v", rootExport.CanonicalName, rootExport.ArtifactRefs)
		}
	}

	nodeByID := map[string]graph.Node{}
	for _, node := range run.snapshot.Snapshot.Nodes {
		nodeByID[node.ID] = node
	}
	for _, root := range run.roots {
		node, ok := nodeByID[root.ID]
		if !ok {
			t.Fatalf("expected endpoint node for root %+v", root)
		}
		if node.Kind != graph.NodeEndpoint {
			t.Fatalf("expected endpoint node kind for %s, got %s", node.ID, node.Kind)
		}
		if node.RepositoryID == "" || node.FilePath == "" {
			t.Fatalf("expected persisted endpoint metadata on %+v", node)
		}
		if node.Properties["source_start_byte"] == "" || node.Properties["source_end_byte"] == "" {
			t.Fatalf("expected source span properties on %+v", node)
		}
	}

	edges := boundaryEdges(run.snapshot.Snapshot.Edges)
	if len(edges) == 0 {
		t.Fatal("expected persisted REGISTERS_BOUNDARY edges")
	}
	for _, root := range run.roots {
		if root.HandlerTarget == "" {
			continue
		}
		edge, ok := registerEdgeFrom(edges, root.ID)
		if !ok {
			t.Fatalf("expected REGISTERS_BOUNDARY edge from %s", root.ID)
		}
		if edge.From != root.ID {
			t.Fatalf("expected boundary edge from %s, got %+v", root.ID, edge)
		}
	}
}

func rootByCanonicalNameOrFail(t *testing.T, roots []boundaryroot.Root, canonical string) boundaryroot.Root {
	t.Helper()
	for _, root := range roots {
		if root.CanonicalName == canonical {
			return root
		}
	}
	t.Fatalf("root %q not found in %+v", canonical, roots)
	return boundaryroot.Root{}
}

func rootExportSlugs(exports []export_mermaid.RootExport) []string {
	slugs := make([]string, 0, len(exports))
	for _, rootExport := range exports {
		slugs = append(slugs, rootExport.Slug)
	}
	return slugs
}
