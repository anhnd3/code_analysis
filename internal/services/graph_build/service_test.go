package graph_build

import (
	"testing"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
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
	result := builder.Build("ws_demo", "snap_demo", inventory, extraction)
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
