package integration

import (
	"slices"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/graph"
	queryport "analysis-module/internal/ports/query"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
)

func TestBuildSnapshotCapturesRelationQualityAcrossPythonAndTypeScript(t *testing.T) {
	app := newGraphTestApplication(t)
	result, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "relation_quality_app"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}

	expectedCalls := [][2]string{
		{"py.app.handle_request", "py.worker.process_data"},
		{"py.worker.process_data", "py.helpers.finalize_data"},
		{"web.renderPage", "web.helpers.formatMessage"},
		{"web.renderPage", "web.helpers.normalize"},
		{"web.renderPage", "web.helpers.defaultFormatter"},
	}
	for _, pair := range expectedCalls {
		if !hasCall(result.Snapshot.Edges, result.Snapshot.Nodes, pair[0], pair[1]) {
			t.Fatalf("expected CALLS edge %s -> %s", pair[0], pair[1])
		}
	}

	if !hasTestedBy(result.Snapshot.Edges, result.Snapshot.Nodes, "py.app.handle_request", "py.test_app.test_handle_request") {
		t.Fatal("expected TESTED_BY edge from py.app.handle_request to py.test_app.test_handle_request")
	}
	if !hasTestedByToFile(result.Snapshot.Edges, result.Snapshot.Nodes, "web.renderPage", "web/index.test.ts") {
		t.Fatal("expected TESTED_BY edge from web.renderPage into web/index.test.ts")
	}

	if result.QualityReport.IssueCounts.UnresolvedImports != 0 {
		t.Fatalf("expected no unresolved imports, got %d", result.QualityReport.IssueCounts.UnresolvedImports)
	}
	if result.QualityReport.IssueCounts.AmbiguousRelations != 0 {
		t.Fatalf("expected no ambiguous relations, got %d", result.QualityReport.IssueCounts.AmbiguousRelations)
	}
}

func TestImpactedTestsUsesRelationEdgesInsteadOfStructuralNoise(t *testing.T) {
	app := newGraphTestApplication(t)
	snapshot, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "relation_quality_app"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	result, err := app.ImpactedTests.Run(impacted_tests.Request{
		WorkspaceID: snapshot.WorkspaceID,
		SnapshotID:  snapshot.Snapshot.ID,
		Target:      "web.helpers.normalize",
		MaxDepth:    4,
	})
	if err != nil {
		t.Fatalf("impacted tests: %v", err)
	}
	files := impactedFiles(result.QueryResult.Tests)
	if !slices.Contains(files, "web/index.test.ts") {
		t.Fatalf("expected impacted tests to include web/index.test.ts, got %v", files)
	}
}

func TestIgnorePolicyRemovesIgnoredFilesFromGraphAndQuality(t *testing.T) {
	app := newGraphTestApplication(t)
	result, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath:  fixtures.WorkspacePath(t, "ignore_policy_app"),
		IgnorePatterns: []string{"ignored"},
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	for _, node := range result.Snapshot.Nodes {
		if node.FilePath == "ignored/server.ts" {
			t.Fatal("expected ignored file to be excluded from graph nodes")
		}
	}
	if result.QualityReport.IssueCounts.SkippedIgnoredFiles == 0 {
		t.Fatal("expected skipped ignored files to be reported")
	}
}

func TestMultiServiceSharedCodeGetsSharedOwnership(t *testing.T) {
	app := newGraphTestApplication(t)
	result, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: fixtures.WorkspacePath(t, "multi_service_shared_repo"),
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	serviceNames := serviceNodeNames(result.Snapshot.Nodes)
	if !slices.Contains(serviceNames, "api") || !slices.Contains(serviceNames, "worker") {
		t.Fatalf("expected api and worker services, got %v", serviceNames)
	}
	ownerEdges := belongsToServiceEdges(result.Snapshot.Edges, result.Snapshot.Nodes, "util.Shared")
	if len(ownerEdges) != 2 {
		t.Fatalf("expected shared util.Shared to belong to 2 services, got %d", len(ownerEdges))
	}
	for _, edge := range ownerEdges {
		if edge.Properties["shared"] != "true" {
			t.Fatalf("expected shared ownership edge, got properties=%v", edge.Properties)
		}
	}
}

func newGraphTestApplication(t *testing.T) *bootstrap.Application {
	t.Helper()
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = cfg.ArtifactRoot + "/analysis.sqlite"
	cfg.ProgressMode = "quiet"
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return app
}

func hasCall(edges []graph.Edge, nodes []graph.Node, fromCanonical, toCanonical string) bool {
	ids := nodeIDsByCanonical(nodes)
	fromID := ids[fromCanonical]
	toID := ids[toCanonical]
	for _, edge := range edges {
		if edge.Kind == graph.EdgeCalls && edge.From == fromID && edge.To == toID {
			return true
		}
	}
	return false
}

func hasTestedBy(edges []graph.Edge, nodes []graph.Node, fromCanonical, toCanonical string) bool {
	ids := nodeIDsByCanonical(nodes)
	fromID := ids[fromCanonical]
	toID := ids[toCanonical]
	for _, edge := range edges {
		if edge.Kind == graph.EdgeTestedBy && edge.From == fromID && edge.To == toID {
			return true
		}
	}
	return false
}

func hasTestedByToFile(edges []graph.Edge, nodes []graph.Node, fromCanonical, filePath string) bool {
	ids := nodeIDsByCanonical(nodes)
	fromID := ids[fromCanonical]
	for _, edge := range edges {
		if edge.Kind != graph.EdgeTestedBy || edge.From != fromID {
			continue
		}
		for _, node := range nodes {
			if node.ID == edge.To && node.FilePath == filePath && node.Kind == graph.NodeTest {
				return true
			}
		}
	}
	return false
}

func impactedFiles(entities []queryport.ImpactedEntity) []string {
	files := make([]string, 0, len(entities))
	for _, entity := range entities {
		files = append(files, entity.Node.FilePath)
	}
	return files
}

func nodeIDsByCanonical(nodes []graph.Node) map[string]string {
	ids := map[string]string{}
	for _, node := range nodes {
		if node.CanonicalName != "" {
			ids[node.CanonicalName] = node.ID
		}
	}
	return ids
}

func serviceNodeNames(nodes []graph.Node) []string {
	names := []string{}
	for _, node := range nodes {
		if node.Kind == graph.NodeService {
			names = append(names, node.CanonicalName)
		}
	}
	return names
}

func belongsToServiceEdges(edges []graph.Edge, nodes []graph.Node, canonical string) []graph.Edge {
	ids := nodeIDsByCanonical(nodes)
	targetID := ids[canonical]
	result := []graph.Edge{}
	for _, edge := range edges {
		if edge.Kind == graph.EdgeBelongsToService && edge.From == targetID {
			result = append(result, edge)
		}
	}
	return result
}
