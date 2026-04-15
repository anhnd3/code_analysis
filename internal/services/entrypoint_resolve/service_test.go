package entrypoint_resolve

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
)

func TestResolve_BootstrapMainMain(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n1", "cmd/server/main.main", "function", "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	assertRootFound(t, result, "n1", entrypoint.RootBootstrap, entrypoint.ConfidenceHigh)
}

func TestResolve_BootstrapCmdExecute(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n1", "cmd/root.Execute", "method", "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	assertRootFound(t, result, "n1", entrypoint.RootBootstrap, entrypoint.ConfidenceMedium)
}

func TestResolve_HTTPHandler(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n1", "api/handler.GetUser", string(symbol.KindRouteHandler), "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	assertRootFound(t, result, "n1", entrypoint.RootHTTP, entrypoint.ConfidenceHigh)
}

func TestResolve_GRPCHandler(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n1", "internal/grpc.CreateOrder", string(symbol.KindGRPCHandler), "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	assertRootFound(t, result, "n1", entrypoint.RootGRPC, entrypoint.ConfidenceHigh)
}

func TestResolve_Consumer(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n1", "worker/handler.ProcessEvent", string(symbol.KindConsumer), "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	assertRootFound(t, result, "n1", entrypoint.RootConsumer, entrypoint.ConfidenceHigh)
}

func TestResolve_CLIFromEntrypointEdge(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "svc1", Kind: graph.NodeService, CanonicalName: "my-service"},
			{ID: "f1", Kind: graph.NodeFile, CanonicalName: "cmd/main.go", RepositoryID: "repo1"},
		},
		Edges: []graph.Edge{
			{ID: "e1", Kind: graph.EdgeEntrypointTo, From: "svc1", To: "f1"},
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	assertRootFound(t, result, "f1", entrypoint.RootCLI, entrypoint.ConfidenceHigh)
}

func TestResolve_DeterministicOrder(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n2", "cmd/server/main.main", "function", "repo1"),
			symbolNode("n1", "api/handler.GetUser", string(symbol.KindRouteHandler), "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Roots) < 2 {
		t.Fatalf("expected at least 2 roots, got %d", len(result.Roots))
	}
	for i := 1; i < len(result.Roots); i++ {
		if result.Roots[i].CanonicalName < result.Roots[i-1].CanonicalName {
			t.Errorf("roots not sorted: %s before %s", result.Roots[i-1].CanonicalName, result.Roots[i].CanonicalName)
		}
	}
}

func TestResolve_Deduplicate(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symbolNode("n1", "cmd/server/main.main", "function", "repo1"),
			symbolNode("n1", "cmd/server/main.main", "function", "repo1"),
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range result.Roots {
		if r.NodeID == "n1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 deduplicated root, got %d", count)
	}
}

func TestResolve_NoRoots(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "n1", Kind: graph.NodePackage, CanonicalName: "utils"},
		},
	}
	result, err := New().Resolve(snapshot, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Roots) != 0 {
		t.Errorf("expected 0 roots, got %d", len(result.Roots))
	}
}

// --- test helpers ---

func symbolNode(id, canonical, kind, repoID string) graph.Node {
	return graph.Node{
		ID:            id,
		Kind:          graph.NodeSymbol,
		CanonicalName: canonical,
		RepositoryID:  repoID,
		Properties: map[string]string{
			"kind": kind,
			"name": shortSymbolName(canonical),
		},
	}
}

func assertRootFound(t *testing.T, result entrypoint.Result, nodeID string, rootType entrypoint.RootType, confidence entrypoint.Confidence) {
	t.Helper()
	for _, r := range result.Roots {
		if r.NodeID == nodeID {
			if r.RootType != rootType {
				t.Errorf("root %s: expected type %s, got %s", nodeID, rootType, r.RootType)
			}
			if r.Confidence != confidence {
				t.Errorf("root %s: expected confidence %s, got %s", nodeID, confidence, r.Confidence)
			}
			return
		}
	}
	t.Errorf("root %s not found in results (have %d roots)", nodeID, len(result.Roots))
}
