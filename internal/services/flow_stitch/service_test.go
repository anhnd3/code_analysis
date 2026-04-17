package flow_stitch

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
)

func TestBuild_BootstrapChain(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "app.NewServer", "function"),
			testSymbolNode("n3", "app.Server.Start", "method"),
		},
		Edges: []graph.Edge{
			callEdge("e1", "n1", "n2"),
			callEdge("e2", "n2", "n3"),
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", CanonicalName: "main.main", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(bundle.Chains))
	}
	chain := bundle.Chains[0]
	if chain.RootNodeID != "n1" {
		t.Errorf("expected root n1, got %s", chain.RootNodeID)
	}
	if len(chain.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(chain.Steps))
	}
}

func TestBuild_ConstructorDetection(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "app.NewService", "function"),
		},
		Edges: []graph.Edge{
			callEdge("e1", "n1", "n2"),
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 || len(bundle.Chains[0].Steps) != 1 {
		t.Fatalf("expected 1 chain with 1 step")
	}
	if bundle.Chains[0].Steps[0].Kind != flow.StepConstruct {
		t.Errorf("expected construct step, got %s", bundle.Chains[0].Steps[0].Kind)
	}
}

func TestBuild_BoundaryMarkerHTTP(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{
				ID: "n1", Kind: graph.NodeSymbol, CanonicalName: "api.GetUser",
				Properties: map[string]string{"kind": string(symbol.KindRouteHandler), "name": "GetUser"},
			},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootHTTP, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range bundle.BoundaryMarkers {
		if m.NodeID == "n1" && m.Protocol == "http" && m.Role == "server" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected HTTP server boundary marker for n1")
	}
}

func TestBuild_BoundaryMarkerOutboundGRPC(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "client.Call", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e1", Kind: graph.EdgeCalls, From: "n1", To: "n2", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e2", Kind: graph.EdgeCallsGRPC, From: "n2", To: "remote1", Evidence: graph.Evidence{Details: "order.OrderService/CreateOrder"}, Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range bundle.BoundaryMarkers {
		if m.NodeID == "n2" && m.Protocol == "grpc" && m.Role == "client" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gRPC client boundary marker for n2")
	}
}

func TestBuild_CycleProtection(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "a.Foo", "function"),
			testSymbolNode("n2", "a.Bar", "function"),
		},
		Edges: []graph.Edge{
			callEdge("e1", "n1", "n2"),
			callEdge("e2", "n2", "n1"),
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(bundle.Chains))
	}
	// Should not loop forever
	if len(bundle.Chains[0].Steps) != 1 {
		t.Errorf("expected 1 step (cycle broken), got %d", len(bundle.Chains[0].Steps))
	}
}

func TestBuild_InferredEdge(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("n2", "util.Helper", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e1", Kind: graph.EdgeCalls, From: "n1", To: "n2",
				Confidence: graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.6}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains[0].Steps) != 1 {
		t.Fatalf("expected 1 step")
	}
	if !bundle.Chains[0].Steps[0].Inferred {
		t.Errorf("expected step to be marked as inferred")
	}
}

func TestBuild_EmptySnapshot(t *testing.T) {
	snapshot := graph.GraphSnapshot{}
	roots := entrypoint.Result{}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Chains) != 0 {
		t.Errorf("expected 0 chains, got %d", len(bundle.Chains))
	}
}

func TestBuild_ExecutionOrder(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			testSymbolNode("n1", "main.main", "function"),
			testSymbolNode("call_target", "app.Call", "function"),
			testSymbolNode("branch_target", "app.Branch", "function"),
			testSymbolNode("spawn_target", "app.Spawn", "function"),
			testSymbolNode("wait_target", "app.Wait", "function"),
			testSymbolNode("defer_target", "app.Defer", "function"),
		},
		Edges: []graph.Edge{
			{ID: "e_defer", Kind: graph.EdgeDefers, From: "n1", To: "defer_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_wait", Kind: graph.EdgeWaitsOn, From: "n1", To: "wait_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_spawn", Kind: graph.EdgeSpawns, From: "n1", To: "spawn_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_branch", Kind: graph.EdgeBranches, From: "n1", To: "branch_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
			{ID: "e_call", Kind: graph.EdgeCalls, From: "n1", To: "call_target", Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed}},
		},
	}
	roots := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "n1", RootType: entrypoint.RootBootstrap, RepositoryID: "repo1"},
		},
	}

	bundle, err := New().Build(snapshot, roots, repository.Inventory{})
	if err != nil {
		t.Fatal(err)
	}

	if len(bundle.Chains) != 1 {
		t.Fatalf("expected 1 chain")
	}
	steps := bundle.Chains[0].Steps
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	// Order: Call (0), Branch (1), Spawn (2), Wait (3), Defer (4)
	expectedOrders := []flow.StepKind{
		flow.StepCall,
		flow.StepBranch,
		flow.StepAsync,
		flow.StepWait,
		flow.StepDefer,
	}

	for i, expectedKind := range expectedOrders {
		if steps[i].Kind != expectedKind {
			t.Errorf("step %d: expected kind %s, got %s", i, expectedKind, steps[i].Kind)
		}
	}
}

// --- test helpers ---

func testSymbolNode(id, canonical, kind string) graph.Node {
	name := canonical
	if idx := len(canonical) - 1; idx >= 0 {
		for i := len(canonical) - 1; i >= 0; i-- {
			if canonical[i] == '.' {
				name = canonical[i+1:]
				break
			}
		}
	}
	return graph.Node{
		ID:            id,
		Kind:          graph.NodeSymbol,
		CanonicalName: canonical,
		RepositoryID:  "repo1",
		Properties: map[string]string{
			"kind": kind,
			"name": name,
		},
	}
}

func callEdge(id, from, to string) graph.Edge {
	return graph.Edge{
		ID:         id,
		Kind:       graph.EdgeCalls,
		From:       from,
		To:         to,
		Confidence: graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.95},
	}
}
