package chain_reduce

import (
	"testing"

	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
)

func TestReduce_BasicChain(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "app.NewServer", "function"),
			symNode("n3", "app.Server.Start", "method"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID:   "n1",
				RepositoryID: "repo1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepConstruct, Label: "NewServer"},
					{FromNodeID: "n2", ToNodeID: "n3", Kind: flow.StepCall, Label: "Start"},
				},
			},
		},
	}

	chain, err := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{})
	if err != nil {
		t.Fatal(err)
	}

	if chain.RootNodeID != "n1" {
		t.Errorf("expected root n1, got %s", chain.RootNodeID)
	}
	if len(chain.Nodes) < 3 {
		t.Errorf("expected at least 3 nodes, got %d", len(chain.Nodes))
	}
	if len(chain.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(chain.Edges))
	}
}

func TestReduce_HelperCollapse(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "svc.Handle", "function"),
			symNode("n3", "util.getString", "function"),
			symNode("n4", "repo.Save", "function"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepCall, Label: "Handle"},
					{FromNodeID: "n2", ToNodeID: "n3", Kind: flow.StepCall, Label: "getString"},
					{FromNodeID: "n2", ToNodeID: "n4", Kind: flow.StepCall, Label: "Save"},
				},
			},
		},
	}

	chain, err := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{CollapseMode: "default"})
	if err != nil {
		t.Fatal(err)
	}

	// "getString" is a tiny helper (starts with "get", less than 12 chars) — should be collapsed
	for _, n := range chain.Nodes {
		if n.ID == "n3" {
			t.Errorf("tiny helper 'getString' should have been collapsed")
		}
	}

	// "Save" should be kept (not a tiny helper)
	found := false
	for _, n := range chain.Nodes {
		if n.ID == "n4" {
			found = true
		}
	}
	if !found {
		t.Errorf("node 'Save' should have been kept")
	}
}

func TestReduce_NoCollapseMode(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "util.getString", "function"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepCall, Label: "getString"},
				},
			},
		},
	}

	chain, err := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{CollapseMode: "none"})
	if err != nil {
		t.Fatal(err)
	}

	// With "none" mode, even tiny helpers should be kept
	found := false
	for _, n := range chain.Nodes {
		if n.ID == "n2" {
			found = true
		}
	}
	if !found {
		t.Errorf("node 'getString' should be kept in 'none' collapse mode")
	}
}

func TestReduce_DepthLimit(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "a.Process", "function"),
			symNode("n3", "b.Process", "function"),
			symNode("n4", "c.Process", "function"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepCall, Label: "Process"},
					{FromNodeID: "n2", ToNodeID: "n3", Kind: flow.StepCall, Label: "Process"},
					{FromNodeID: "n3", ToNodeID: "n4", Kind: flow.StepCall, Label: "Process"},
				},
			},
		},
	}

	chain, err := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{MaxDepth: 2, CollapseMode: "none"})
	if err != nil {
		t.Fatal(err)
	}

	// Should stop at depth 2 — only n1, n2, n3 (root + 2 depth)
	if len(chain.Edges) > 2 {
		t.Errorf("expected at most 2 edges with depth 2, got %d", len(chain.Edges))
	}
}

func TestReduce_InferredNote(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "svc.Handle", "function"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepCall, Label: "Handle", Inferred: true},
				},
			},
		},
	}

	chain, err := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{CollapseMode: "none"})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, note := range chain.Notes {
		if note.AtNodeID == "n2" && note.Kind == "inference" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inference note for inferred edge")
	}
}

func TestReduce_CrossRepoCrossing_Confirmed(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "n1", Kind: graph.NodeSymbol, CanonicalName: "a.Call", RepositoryID: "repo_a",
				Properties: map[string]string{"kind": "function", "name": "Call"}},
			{ID: "n2", Kind: graph.NodeSymbol, CanonicalName: "b.Handle", RepositoryID: "repo_b",
				Properties: map[string]string{"kind": "grpc_handler", "name": "Handle"}},
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepBoundary, Label: "Handle"},
				},
			},
		},
	}
	links := boundary.Bundle{
		Links: []boundary.Link{
			{OutboundNodeID: "n1", InboundNodeID: "n2", Protocol: boundary.ProtocolGRPC,
				Status: boundary.StatusConfirmed, OutboundRepoID: "repo_a", InboundRepoID: "repo_b"},
		},
	}

	chain, err := New().Reduce(snapshot, flows, links, Request{CollapseMode: "none"})
	if err != nil {
		t.Fatal(err)
	}

	// Confirmed cross should produce a cross-repo edge
	found := false
	for _, e := range chain.Edges {
		if e.FromID == "n1" && e.ToID == "n2" && e.CrossRepo {
			found = true
			if e.LinkStatus != string(boundary.StatusConfirmed) {
				t.Errorf("expected confirmed link status, got %s", e.LinkStatus)
			}
		}
	}
	if !found {
		t.Errorf("expected cross-repo edge")
	}
}

func TestReduce_CrossRepoCrossing_Mismatch(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "n1", Kind: graph.NodeSymbol, CanonicalName: "a.Call", RepositoryID: "repo_a",
				Properties: map[string]string{"kind": "function", "name": "Call"}},
			{ID: "n2", Kind: graph.NodeSymbol, CanonicalName: "b.Handle", RepositoryID: "repo_b",
				Properties: map[string]string{"kind": "grpc_handler", "name": "Handle"}},
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepBoundary, Label: "Handle"},
				},
			},
		},
	}
	links := boundary.Bundle{
		Links: []boundary.Link{
			{OutboundNodeID: "n1", InboundNodeID: "n2", Protocol: boundary.ProtocolGRPC,
				Status: boundary.StatusMismatch, OutboundRepoID: "repo_a", InboundRepoID: "repo_b"},
		},
	}

	chain, err := New().Reduce(snapshot, flows, links, Request{CollapseMode: "none"})
	if err != nil {
		t.Fatal(err)
	}

	// Mismatch should NOT produce a cross-repo edge but SHOULD produce a warning note
	for _, e := range chain.Edges {
		if e.FromID == "n1" && e.ToID == "n2" {
			t.Errorf("mismatch should not produce a cross-repo edge")
		}
	}
	found := false
	for _, note := range chain.Notes {
		if note.Kind == "candidate" && note.AtNodeID == "n1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning note for mismatch")
	}
}

func TestReduce_ConstructorKept(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "app.NewServer", "function"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepConstruct, Label: "NewServer"},
				},
			},
		},
	}

	chain, err := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, n := range chain.Nodes {
		if n.ID == "n2" && n.Role == reduced.RoleConstructor {
			found = true
		}
	}
	if !found {
		t.Errorf("constructors should be kept, not collapsed")
	}
}

func TestReduce_EmptyInput(t *testing.T) {
	chain, err := New().Reduce(graph.GraphSnapshot{}, flow.Bundle{}, boundary.Bundle{}, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if chain.RootNodeID != "" {
		t.Errorf("expected empty chain for empty input")
	}
}

func TestReduce_DeterministicOutput(t *testing.T) {
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			symNode("n1", "main.main", "function"),
			symNode("n2", "svc.Handle", "function"),
			symNode("n3", "repo.Save", "function"),
		},
	}
	flows := flow.Bundle{
		Chains: []flow.Chain{
			{
				RootNodeID: "n1",
				Steps: []flow.Step{
					{FromNodeID: "n1", ToNodeID: "n3", Kind: flow.StepCall, Label: "Save"},
					{FromNodeID: "n1", ToNodeID: "n2", Kind: flow.StepCall, Label: "Handle"},
				},
			},
		},
	}

	chain1, _ := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{CollapseMode: "none"})
	chain2, _ := New().Reduce(snapshot, flows, boundary.Bundle{}, Request{CollapseMode: "none"})

	if len(chain1.Nodes) != len(chain2.Nodes) {
		t.Fatalf("non-deterministic node count: %d vs %d", len(chain1.Nodes), len(chain2.Nodes))
	}
	for i := range chain1.Nodes {
		if chain1.Nodes[i].ID != chain2.Nodes[i].ID {
			t.Errorf("non-deterministic node order at %d: %s vs %s", i, chain1.Nodes[i].ID, chain2.Nodes[i].ID)
		}
	}
}

// --- helpers ---

func symNode(id, canonical, kind string) graph.Node {
	name := canonical
	for i := len(canonical) - 1; i >= 0; i-- {
		if canonical[i] == '.' {
			name = canonical[i+1:]
			break
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
