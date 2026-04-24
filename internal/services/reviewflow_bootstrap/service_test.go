package reviewflow_bootstrap

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reviewflow"
)

func TestBuild_BootstrapLifecycleFromEvidence(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "main_root",
		CanonicalName: "main.main",
		RootType:      entrypoint.RootBootstrap,
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "main_root", CanonicalName: "main.main", Kind: graph.NodeSymbol},
			{ID: "cfg", CanonicalName: "cmd.loadConfig", Kind: graph.NodeSymbol},
			{ID: "deps", CanonicalName: "cmd.initServer", Kind: graph.NodeSymbol},
			{ID: "router", CanonicalName: "cmd.withRouter", Kind: graph.NodeSymbol},
			{ID: "server", CanonicalName: "service.startHTTP", Kind: graph.NodeSymbol},
		},
		Edges: []graph.Edge{
			{ID: "e_cfg", Kind: graph.EdgeCalls, From: "main_root", To: "cfg"},
			{ID: "e_deps", Kind: graph.EdgeCalls, From: "main_root", To: "deps"},
			{ID: "e_router", Kind: graph.EdgeCalls, From: "deps", To: "router"},
			{ID: "e_server", Kind: graph.EdgeCalls, From: "router", To: "server"},
		},
	}

	flow, err := service.Build(snapshot, root)
	if err != nil {
		t.Fatalf("build bootstrap flow: %v", err)
	}
	if flow.Metadata.CandidateKind != "bootstrap_lifecycle" {
		t.Fatalf("expected bootstrap lifecycle candidate, got %+v", flow.Metadata)
	}
	if len(flow.Stages) < 4 {
		t.Fatalf("expected lifecycle stages, got %+v", flow.Stages)
	}
	if !hasStage(flow, stageProcessEntry) || !hasStage(flow, stageServeLoop) {
		t.Fatalf("expected process entry and serve loop stages, got %+v", flow.Stages)
	}
}

func TestBuild_FailsWhenNoUsableEvidence(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "main_root",
		CanonicalName: "main.main",
		RootType:      entrypoint.RootBootstrap,
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "main_root", CanonicalName: "main.main", Kind: graph.NodeSymbol},
		},
	}

	_, err := service.Build(snapshot, root)
	if err == nil {
		t.Fatal("expected insufficient bootstrap evidence failure")
	}
	if err != ErrInsufficientEvidence {
		t.Fatalf("expected ErrInsufficientEvidence, got %v", err)
	}
}

func TestBuild_UsesRootEvidenceWhenLifecycleEdgesMissing(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "main_root",
		CanonicalName: "main.main",
		RootType:      entrypoint.RootBootstrap,
		Evidence:      "main.main pattern",
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "main_root", CanonicalName: "main.main", Kind: graph.NodeSymbol},
		},
	}

	flow, err := service.Build(snapshot, root)
	if err != nil {
		t.Fatalf("expected root-evidence fallback to render bootstrap flow, got %v", err)
	}
	if len(flow.Stages) == 0 {
		t.Fatalf("expected lifecycle stages, got %+v", flow.Stages)
	}
}

func hasStage(flow reviewflow.Flow, kind string) bool {
	for _, stage := range flow.Stages {
		if stage.Kind == kind {
			return true
		}
	}
	return false
}
