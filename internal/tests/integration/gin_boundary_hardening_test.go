package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/sequence"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
)

func TestGinBoundaryHardeningPersistsStableEndpointsAndCleansExports(t *testing.T) {
	app := newGraphTestApplication(t)
	workspace := fixtures.WorkspacePath(t, "gin_boundary_hardening")

	first := runGinBoundaryHardeningE2E(t, app, workspace)
	assertGinBoundaryHardeningResult(t, first)

	waitForNextSnapshotIDTick()

	second := runGinBoundaryHardeningE2E(t, app, workspace)
	assertGinBoundaryHardeningResult(t, second)

	firstRootIDs := boundaryRootIDs(first.roots)
	secondRootIDs := boundaryRootIDs(second.roots)
	if !slices.Equal(firstRootIDs, secondRootIDs) {
		t.Fatalf("expected stable boundary root IDs across repeated runs:\nfirst=%v\nsecond=%v", firstRootIDs, secondRootIDs)
	}

	firstEndpointSignatures := endpointNodeSignatures(first.snapshot.Snapshot.Nodes)
	secondEndpointSignatures := endpointNodeSignatures(second.snapshot.Snapshot.Nodes)
	if !slices.Equal(firstEndpointSignatures, secondEndpointSignatures) {
		t.Fatalf("expected stable endpoint node identity across repeated runs:\nfirst=%v\nsecond=%v", firstEndpointSignatures, secondEndpointSignatures)
	}

	firstBoundaryEdges := boundaryEdgeSignatures(first.snapshot.Snapshot.Edges)
	secondBoundaryEdges := boundaryEdgeSignatures(second.snapshot.Snapshot.Edges)
	if !slices.Equal(firstBoundaryEdges, secondBoundaryEdges) {
		t.Fatalf("expected stable REGISTERS_BOUNDARY edge identity across repeated runs:\nfirst=%v\nsecond=%v", firstBoundaryEdges, secondBoundaryEdges)
	}
}

type ginBoundaryHardeningRun struct {
	snapshot build_snapshot.Result
	roots    []boundaryroot.Root
	flow     flow.Bundle
	sequence sequence.Diagram
}

func runGinBoundaryHardeningE2E(t *testing.T, app *bootstrap.Application, workspace string) ginBoundaryHardeningRun {
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

	var flowBundle flow.Bundle
	readJSONFile(t, filepath.Join(debugDir, "flow_bundle.json"), &flowBundle)

	var diagram sequence.Diagram
	readJSONFile(t, filepath.Join(debugDir, "sequence_model.json"), &diagram)

	return ginBoundaryHardeningRun{
		snapshot: buildResult,
		roots:    roots,
		flow:     flowBundle,
		sequence: diagram,
	}
}

func assertGinBoundaryHardeningResult(t *testing.T, result ginBoundaryHardeningRun) {
	t.Helper()

	byCanonical := map[string]boundaryroot.Root{}
	for _, root := range result.roots {
		byCanonical[root.CanonicalName] = root
		if strings.HasPrefix(root.CanonicalName, "Any /") {
			t.Fatalf("expected false-positive Any roots to be removed, got %+v", root)
		}
	}

	requiredCanonicals := []string{
		"GET /health",
		"GET /v1/camera/config/all",
		"POST /v1/camera/detect-qr",
		"GET /v1/camera/abtest",
		"POST /v2/camera/detect-qr",
		"GET /metrics/prom",
	}
	for _, canonical := range requiredCanonicals {
		if _, ok := byCanonical[canonical]; !ok {
			t.Fatalf("expected boundary root %q in %+v", canonical, result.roots)
		}
	}

	v1 := byCanonical["POST /v1/camera/detect-qr"]
	v2 := byCanonical["POST /v2/camera/detect-qr"]
	if v1.ID == v2.ID {
		t.Fatalf("expected distinct stable IDs for v1/v2 detect-qr routes, got %q", v1.ID)
	}

	nodeByID := map[string]graph.Node{}
	for _, node := range result.snapshot.Snapshot.Nodes {
		nodeByID[node.ID] = node
	}

	for _, root := range result.roots {
		node, ok := nodeByID[root.ID]
		if !ok {
			t.Fatalf("expected endpoint node %s for root %+v", root.ID, root)
		}
		if node.Kind != graph.NodeEndpoint {
			t.Fatalf("expected endpoint node kind for %s, got %s", node.ID, node.Kind)
		}
		if node.RepositoryID != root.RepositoryID || node.RepositoryID == "" {
			t.Fatalf("expected endpoint node repo %q, got %q", root.RepositoryID, node.RepositoryID)
		}
		if node.FilePath != root.SourceFile || node.FilePath == "" {
			t.Fatalf("expected endpoint node file %q, got %q", root.SourceFile, node.FilePath)
		}
		if node.Properties["source_start_byte"] == "" || node.Properties["source_end_byte"] == "" {
			t.Fatalf("expected source span properties on endpoint node %+v", node)
		}
		if root.ID != boundaryroot.StableID(root) {
			t.Fatalf("expected root ID %q to match stable identity", root.ID)
		}
	}

	registerEdges := boundaryEdges(result.snapshot.Snapshot.Edges)
	if len(registerEdges) < len(result.roots) {
		t.Fatalf("expected registers-boundary edges for each root, got %d edges for %d roots", len(registerEdges), len(result.roots))
	}

	for _, root := range result.roots {
		edge, ok := registerEdgeFrom(registerEdges, root.ID)
		if !ok {
			t.Fatalf("expected REGISTERS_BOUNDARY edge from root %s", root.ID)
		}
		if edge.From != root.ID {
			t.Fatalf("expected edge from %s, got %+v", root.ID, edge)
		}
		if !nodeExistsByID(result.snapshot.Snapshot.Nodes)[edge.To] && !strings.HasPrefix(edge.To, "unresolved_") {
			t.Fatalf("expected REGISTERS_BOUNDARY edge target to be resolved node or unresolved placeholder, got %+v", edge)
		}
	}

	for _, chain := range result.flow.Chains {
		rootNode, ok := nodeByID[chain.RootNodeID]
		if ok && strings.HasPrefix(rootNode.CanonicalName, "Any /") {
			t.Fatalf("expected flow roots to exclude false-positive Any boundaries, got %+v", chain)
		}
	}

	participantLabels := participantLabels(result.sequence)
	if slices.Contains(participantLabels, "Any /zlp_token") || slices.Contains(participantLabels, "Any /error") {
		t.Fatalf("expected sequence model to exclude false-positive Any participants, got %v", participantLabels)
	}
}

func boundaryRootIDs(roots []boundaryroot.Root) []string {
	ids := make([]string, 0, len(roots))
	for _, root := range roots {
		ids = append(ids, root.ID)
	}
	slices.Sort(ids)
	return ids
}

func endpointNodeSignatures(nodes []graph.Node) []string {
	signatures := []string{}
	for _, node := range nodes {
		if node.Kind != graph.NodeEndpoint {
			continue
		}
		signatures = append(signatures, strings.Join([]string{
			node.ID,
			node.RepositoryID,
			node.FilePath,
			node.Properties["source_start_byte"],
			node.Properties["source_end_byte"],
			node.CanonicalName,
		}, "|"))
	}
	slices.Sort(signatures)
	return signatures
}

func boundaryEdges(edges []graph.Edge) []graph.Edge {
	result := []graph.Edge{}
	for _, edge := range edges {
		if edge.Kind == graph.EdgeRegistersBoundary {
			result = append(result, edge)
		}
	}
	return result
}

func boundaryEdgeSignatures(edges []graph.Edge) []string {
	signatures := []string{}
	for _, edge := range edges {
		if edge.Kind != graph.EdgeRegistersBoundary {
			continue
		}
		signatures = append(signatures, strings.Join([]string{edge.ID, edge.From, edge.To}, "|"))
	}
	slices.Sort(signatures)
	return signatures
}

func registerEdgeFrom(edges []graph.Edge, from string) (graph.Edge, bool) {
	for _, edge := range edges {
		if edge.From == from {
			return edge, true
		}
	}
	return graph.Edge{}, false
}

func participantLabels(diagram sequence.Diagram) []string {
	labels := make([]string, 0, len(diagram.Participants))
	for _, participant := range diagram.Participants {
		labels = append(labels, participant.Label)
	}
	return labels
}

func readJSONFile(t *testing.T, path string, dest any) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}
