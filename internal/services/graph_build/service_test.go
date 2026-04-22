package graph_build

import (
	"testing"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/pkg/ids"
)

func TestBuildCreatesCallAndTestEdges(t *testing.T) {
	builder := New(progress.NoopReporter{})
	inventory := repository.Inventory{
		WorkspaceID: "ws_demo",
		Repositories: []repository.Manifest{{
			ID:                "repo_demo",
			Name:              "demo",
			RootPath:          "/demo",
			Role:              repository.RoleService,
			CandidateServices: []service.Manifest{{ID: "svc_demo", Name: "demo"}},
		}},
	}
	extraction := symbol_index.Result{
		Repositories: []symbol_index.RepositoryExtraction{{
			Repository: inventory.Repositories[0],
			Files: []symbol.FileExtractionResult{{
				FilePath:    "service.go",
				PackageName: "demo",
				Symbols: []symbol.Symbol{
					{ID: "sym_handle", RepositoryID: "repo_demo", FilePath: "service.go", PackageName: "demo", Name: "Handle", CanonicalName: "demo.Handle", Kind: symbol.KindFunction},
					{ID: "sym_transform", RepositoryID: "repo_demo", FilePath: "service.go", PackageName: "demo", Name: "Transform", CanonicalName: "demo.Transform", Kind: symbol.KindFunction},
					{ID: "sym_test", RepositoryID: "repo_demo", FilePath: "service_test.go", PackageName: "demo", Name: "TestHandle", CanonicalName: "demo.TestHandle", Kind: symbol.KindTestFunction},
				},
				Relations: []symbol.RelationCandidate{
					{SourceSymbolID: "sym_handle", TargetCanonicalName: "demo.Transform", Relationship: "calls", EvidenceSource: "Transform", ExtractionMethod: "unit", EvidenceType: "identifier"},
					{SourceSymbolID: "sym_test", TargetCanonicalName: "demo.Handle", Relationship: "calls", EvidenceSource: "Handle", ExtractionMethod: "unit", EvidenceType: "identifier"},
				},
			}},
		}},
	}
	result := builder.Build("ws1", "snap1", inventory, extraction, nil)
	if result.Snapshot.Metadata.SymbolCount != 3 {
		t.Fatalf("expected 3 symbols, got %d", result.Snapshot.Metadata.SymbolCount)
	}
	foundCall := false
	foundTestedBy := false
	for _, edge := range result.Snapshot.Edges {
		if edge.Kind == "CALLS" {
			foundCall = true
		}
		if edge.Kind == "TESTED_BY" {
			foundTestedBy = true
		}
	}
	if !foundCall {
		t.Fatal("expected CALLS edge")
	}
	if !foundTestedBy {
		t.Fatal("expected TESTED_BY edge")
	}
}

func TestBuildCarriesRelationOrderIndexOntoCallEdges(t *testing.T) {
	builder := New(progress.NoopReporter{})
	inventory := repository.Inventory{
		WorkspaceID: "ws_demo",
		Repositories: []repository.Manifest{{
			ID:                "repo_demo",
			Name:              "demo",
			RootPath:          "/demo",
			Role:              repository.RoleService,
			CandidateServices: []service.Manifest{{ID: "svc_demo", Name: "demo"}},
		}},
	}
	extraction := symbol_index.Result{
		Repositories: []symbol_index.RepositoryExtraction{{
			Repository: inventory.Repositories[0],
			Files: []symbol.FileExtractionResult{{
				FilePath:    "service.go",
				PackageName: "demo",
				Symbols: []symbol.Symbol{
					{ID: "sym_handle", RepositoryID: "repo_demo", FilePath: "service.go", PackageName: "demo", Name: "Handle", CanonicalName: "demo.Handle", Kind: symbol.KindFunction},
					{ID: "sym_bind", RepositoryID: "repo_demo", FilePath: "service.go", PackageName: "demo", Name: "Bind", CanonicalName: "demo.Bind", Kind: symbol.KindFunction},
				},
				Relations: []symbol.RelationCandidate{
					{
						SourceSymbolID:      "sym_handle",
						TargetCanonicalName: "demo.Bind",
						Relationship:        "calls",
						EvidenceSource:      "Bind",
						ExtractionMethod:    "unit",
						EvidenceType:        "identifier",
						OrderIndex:          2,
					},
				},
			}},
		}},
	}

	result := builder.Build("ws1", "snap1", inventory, extraction, nil)
	for _, edge := range result.Snapshot.Edges {
		if edge.Kind != graph.EdgeCalls {
			continue
		}
		if edge.Properties["order_index"] != "2" {
			t.Fatalf("expected propagated order_index 2, got %+v", edge)
		}
		return
	}
	t.Fatal("expected CALLS edge with order_index")
}

func TestBuildLeavesBoundaryHandlerTargetUnresolvedForSameFileShortName(t *testing.T) {
	builder := New(progress.NoopReporter{})
	inventory := repository.Inventory{
		WorkspaceID: "ws_demo",
		Repositories: []repository.Manifest{{
			ID:                "repo_demo",
			Name:              "demo",
			RootPath:          "/demo",
			Role:              repository.RoleService,
			CandidateServices: []service.Manifest{{ID: "svc_demo", Name: "demo"}},
		}},
	}
	extraction := symbol_index.Result{
		Repositories: []symbol_index.RepositoryExtraction{{
			Repository: inventory.Repositories[0],
			Files: []symbol.FileExtractionResult{{
				FilePath:    "service.go",
				PackageName: "demo",
				Symbols: []symbol.Symbol{
					{ID: "sym_handle", RepositoryID: "repo_demo", FilePath: "service.go", PackageName: "demo", Name: "Handle", CanonicalName: "demo.Handle", Kind: symbol.KindFunction},
				},
			}},
		}},
	}
	root := boundaryroot.Root{
		ID:            "endpoint_any_/users",
		Kind:          boundaryroot.KindHTTP,
		Framework:     "net/http",
		Method:        "ANY",
		Path:          "/users",
		CanonicalName: "ANY /users",
		HandlerTarget: "Handle",
		RepositoryID:  "repo_demo",
		SourceFile:    "service.go",
	}

	result := builder.Build("ws1", "snap1", inventory, extraction, []boundaryroot.Root{root})
	edge := findEdgeByKind(t, result.Snapshot.Edges, graph.EdgeRegistersBoundary)
	if edge.To != "unresolved_Handle" {
		t.Fatalf("expected boundary edge to stay unresolved for short-name target, got %+v", edge)
	}
	if edge.Confidence.Tier != graph.ConfidenceInferred {
		t.Fatalf("expected unresolved boundary target to remain inferred, got %+v", edge.Confidence)
	}
}

func TestBuildLeavesBoundaryHandlerTargetUnresolvedForSamePackageShortName(t *testing.T) {
	builder := New(progress.NoopReporter{})
	inventory := repository.Inventory{
		WorkspaceID: "ws_demo",
		Repositories: []repository.Manifest{{
			ID:                "repo_demo",
			Name:              "demo",
			RootPath:          "/demo",
			Role:              repository.RoleService,
			CandidateServices: []service.Manifest{{ID: "svc_demo", Name: "demo"}},
		}},
	}
	extraction := symbol_index.Result{
		Repositories: []symbol_index.RepositoryExtraction{{
			Repository: inventory.Repositories[0],
			Files: []symbol.FileExtractionResult{
				{
					FilePath:    "routes.go",
					PackageName: "demo",
				},
				{
					FilePath:    "handlers.go",
					PackageName: "demo",
					Symbols: []symbol.Symbol{
						{ID: "sym_handle", RepositoryID: "repo_demo", FilePath: "handlers.go", PackageName: "demo", Name: "Handle", CanonicalName: "demo.Handle", Kind: symbol.KindFunction},
					},
				},
			},
		}},
	}
	root := boundaryroot.Root{
		ID:            "endpoint_any_/users",
		Kind:          boundaryroot.KindHTTP,
		Framework:     "net/http",
		Method:        "ANY",
		Path:          "/users",
		CanonicalName: "ANY /users",
		HandlerTarget: "Handle",
		RepositoryID:  "repo_demo",
		SourceFile:    "routes.go",
	}

	result := builder.Build("ws1", "snap1", inventory, extraction, []boundaryroot.Root{root})
	edge := findEdgeByKind(t, result.Snapshot.Edges, graph.EdgeRegistersBoundary)
	if edge.To != "unresolved_Handle" {
		t.Fatalf("expected boundary edge to stay unresolved for same-package short name, got %+v", edge)
	}
	if edge.Confidence.Tier != graph.ConfidenceInferred {
		t.Fatalf("expected unresolved boundary target to remain inferred, got %+v", edge.Confidence)
	}
}

func findEdgeByKind(t *testing.T, edges []graph.Edge, kind graph.EdgeKind) graph.Edge {
	t.Helper()
	for _, edge := range edges {
		if edge.Kind == kind {
			return edge
		}
	}
	t.Fatalf("expected %s edge", kind)
	return graph.Edge{}
}

func idsForSymbol(symbolID string) string {
	return ids.Stable("node", "symbol", symbolID)
}
