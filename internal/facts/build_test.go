package facts

import (
	"testing"
	"time"

	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
)

func TestBuildIndexResolvesImportSelectorTargetsSafely(t *testing.T) {
	repo := repository.Manifest{
		ID:       "repo-1",
		Name:     "repo",
		RootPath: "/repo",
		Role:     repository.RoleService,
	}

	input := BuildInput{
		WorkspaceID: "ws-1",
		SnapshotID:  "snap-1",
		Inventory: repository.Inventory{
			Repositories: []repository.Manifest{repo},
		},
		Extraction: symbol_index.Result{
			Repositories: []symbol_index.RepositoryExtraction{
				{
					Repository: repo,
					Files: []symbol.FileExtractionResult{
						{
							FilePath:    "root.go",
							Language:    "go",
							PackageName: "demo",
							Symbols: []symbol.Symbol{
								{
									ID:            "sym-root",
									RepositoryID:  "repo-1",
									FilePath:      "root.go",
									PackageName:   "demo",
									Name:          "Root",
									CanonicalName: "demo.Root",
									Kind:          symbol.KindFunction,
								},
							},
							Relations: []symbol.RelationCandidate{
								{
									SourceSymbolID:   "sym-root",
									TargetFilePath:   "resolved.go",
									TargetExportName: "Render",
									Relationship:     "calls",
									EvidenceType:     "static",
									EvidenceSource:   "tree-sitter-go",
									ExtractionMethod: "go",
									ConfidenceScore:  0.93,
									OrderIndex:       0,
								},
								{
									SourceSymbolID:   "sym-root",
									TargetFilePath:   "ambiguous.go",
									TargetExportName: "Render",
									Relationship:     "calls",
									EvidenceType:     "static",
									EvidenceSource:   "tree-sitter-go",
									ExtractionMethod: "go",
									ConfidenceScore:  0.81,
									OrderIndex:       1,
								},
							},
						},
						{
							FilePath:    "resolved.go",
							Language:    "go",
							PackageName: "demo",
							Exports: []symbol.ExportBinding{
								{Name: "Render", CanonicalName: "demo.Render"},
							},
							Symbols: []symbol.Symbol{
								{
									ID:            "sym-resolved",
									RepositoryID:  "repo-1",
									FilePath:      "resolved.go",
									PackageName:   "demo",
									Name:          "Render",
									CanonicalName: "demo.Render",
									Kind:          symbol.KindFunction,
								},
							},
						},
						{
							FilePath:    "ambiguous.go",
							Language:    "go",
							PackageName: "demo",
							Symbols: []symbol.Symbol{
								{
									ID:            "sym-ambiguous-fn",
									RepositoryID:  "repo-1",
									FilePath:      "ambiguous.go",
									PackageName:   "demo",
									Name:          "Render",
									CanonicalName: "demo.Render",
									Kind:          symbol.KindFunction,
								},
								{
									ID:            "sym-ambiguous-method",
									RepositoryID:  "repo-1",
									FilePath:      "ambiguous.go",
									PackageName:   "demo",
									Name:          "Render",
									Receiver:      "renderService",
									CanonicalName: "demo.renderService.Render",
									Kind:          symbol.KindMethod,
								},
							},
						},
					},
				},
			},
		},
		GeneratedAt: time.Unix(1, 0).UTC(),
	}

	index := BuildIndex(input)
	if len(index.CallCandidates) != 2 {
		t.Fatalf("expected 2 call candidates, got %d", len(index.CallCandidates))
	}
	if index.CallCandidates[0].TargetSymbolID != "sym-resolved" {
		t.Fatalf("expected safe resolution for first candidate, got %+v", index.CallCandidates[0])
	}
	if index.CallCandidates[1].TargetSymbolID != "" {
		t.Fatalf("expected ambiguous selector to remain unresolved, got %+v", index.CallCandidates[1])
	}
	if index.IssueCounts.AmbiguousRelations != 1 {
		t.Fatalf("expected ambiguous relation count to be incremented, got %+v", index.IssueCounts)
	}
	if len(index.Diagnostics) == 0 || index.Diagnostics[len(index.Diagnostics)-1].Category != "ambiguous_relation" {
		t.Fatalf("expected ambiguous relation diagnostic, got %+v", index.Diagnostics)
	}
}
