package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"analysis-module/internal/facts"
)

func TestSaveIndexRoundTripsFacts(t *testing.T) {
	store := newTestStore(t)

	index := facts.Index{
		WorkspaceID: "ws-1",
		SnapshotID:  "snap-1",
		GeneratedAt: time.Date(2024, time.January, 2, 3, 4, 5, 6, time.UTC),
		IssueCounts: facts.IssueCountsFact{
			UnresolvedImports:  1,
			AmbiguousRelations: 2,
		},
		Repositories: []facts.RepositoryFact{
			{
				ID:        "repo-1",
				Name:      "repo",
				RootPath:  t.TempDir(),
				Role:      "service",
				Languages: []string{"go"},
			},
		},
		Services: []facts.ServiceFact{
			{
				ID:           "svc-1",
				RepositoryID: "repo-1",
				Name:         "svc",
				RootPath:     "/tmp/repo/svc",
			},
		},
		Files: []facts.FileFact{
			{
				ID:             "file-1",
				RepositoryID:   "repo-1",
				RepositoryRoot: "/tmp/repo",
				RelativePath:   "pkg/file.go",
				AbsolutePath:   "/tmp/repo/pkg/file.go",
				Language:       "go",
				PackageName:    "demo",
			},
		},
		Symbols: []facts.SymbolFact{
			{
				ID:            "sym-1",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "pkg/file.go",
				CanonicalName: "demo.Handle",
				Name:          "Handle",
				Kind:          "function",
				StartLine:     3,
				EndLine:       5,
			},
		},
		Imports: []facts.ImportFact{
			{
				ID:       "imp-1",
				FileID:   "file-1",
				FilePath: "pkg/file.go",
				Source:   "example.com/dep",
				Alias:    "dep",
			},
		},
		Exports: []facts.ExportFact{
			{
				ID:            "exp-1",
				FileID:        "file-1",
				FilePath:      "pkg/file.go",
				Name:          "Handle",
				CanonicalName: "demo.Handle",
			},
		},
		CallCandidates: []facts.CallCandidate{
			{
				ID:                  "call-1",
				SourceSymbolID:      "sym-1",
				TargetSymbolID:      "sym-2",
				TargetCanonicalName: "demo.Child",
				TargetFilePath:      "pkg/child.go",
				TargetExportName:    "Child",
				Relationship:        "calls",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.91,
				OrderIndex:          7,
			},
		},
		ExecutionHints: []facts.ExecutionHintFact{
			{
				ID:             "hint-1",
				FilePath:       "pkg/file.go",
				SourceSymbolID: "sym-1",
				TargetSymbolID: "sym-2",
				TargetSymbol:   "demo.Child",
				Kind:           "spawn",
				Evidence:       "go routine",
				OrderIndex:     0,
			},
		},
		Diagnostics: []facts.DiagnosticFact{
			{
				ID:       "diag-1",
				FilePath: "pkg/file.go",
				SymbolID: "sym-1",
				Category: "example",
				Message:  "note",
				Evidence: "evidence",
			},
		},
		Tests: []facts.TestFact{
			{
				ID:            "test-1",
				SymbolID:      "sym-test",
				FileID:        "file-1",
				FilePath:      "pkg/file.go",
				CanonicalName: "demo.TestHandle",
				Name:          "TestHandle",
				StartLine:     9,
				EndLine:       11,
			},
		},
		Boundaries: []facts.BoundaryFact{
			{
				ID:            "boundary-1",
				RepositoryID:  "repo-1",
				Kind:          "http",
				Framework:     "gin",
				CanonicalName: "demo.HTTP",
				SourceFile:    "pkg/file.go",
				HandlerTarget: "demo.Handle",
			},
		},
	}

	if err := store.SaveIndex(index); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	gotSymbol, err := store.GetSymbolByID(index.WorkspaceID, index.SnapshotID, "sym-1")
	if err != nil {
		t.Fatalf("GetSymbolByID: %v", err)
	}
	if gotSymbol.CanonicalName != "demo.Handle" || gotSymbol.Name != "Handle" {
		t.Fatalf("unexpected symbol round-trip: %+v", gotSymbol)
	}

	imports, err := store.ListImportsByFile(index.WorkspaceID, index.SnapshotID, "file-1")
	if err != nil {
		t.Fatalf("ListImportsByFile: %v", err)
	}
	if len(imports) != 1 || imports[0].Alias != "dep" || imports[0].Source != "example.com/dep" {
		t.Fatalf("unexpected imports round-trip: %+v", imports)
	}

	calls, err := store.ListOutgoingCallCandidates(index.WorkspaceID, index.SnapshotID, "sym-1")
	if err != nil {
		t.Fatalf("ListOutgoingCallCandidates: %v", err)
	}
	if len(calls) != 1 || calls[0].TargetSymbolID != "sym-2" || calls[0].TargetExportName != "Child" {
		t.Fatalf("unexpected call candidate round-trip: %+v", calls)
	}
	if calls[0].OrderIndex != 7 {
		t.Fatalf("unexpected order index: %+v", calls[0])
	}
}

func TestSaveAndGetReviewFlowRoundTripsEvidenceRefs(t *testing.T) {
	store := newTestStore(t)

	createdAt := time.Date(2024, time.March, 4, 5, 6, 7, 8, time.UTC)
	flow := facts.ReviewFlow{
		ID:                "flow-1",
		WorkspaceID:       "ws-1",
		SnapshotID:        "snap-1",
		RootSymbolID:      "sym-1",
		RootCanonicalName: "demo.Handle",
		CreatedAt:         createdAt,
		Steps: []facts.ReviewStep{
			{
				ID:                "step-1",
				FromSymbolID:      "sym-1",
				FromCanonicalName: "demo.Handle",
				ToSymbolID:        "sym-2",
				ToCanonicalName:   "demo.Child",
				Status:            facts.StepAccepted,
				Rationale:         "evidence-backed",
				Evidence: []facts.EvidenceRef{
					{
						SymbolID:  "sym-1",
						FilePath:  "pkg/file.go",
						StartLine: 3,
						EndLine:   5,
						Snippet:   "return child()",
						Source:    "tree-sitter-go",
					},
				},
			},
		},
		Accepted: []facts.ReviewStep{
			{
				ID:                "step-1",
				FromSymbolID:      "sym-1",
				FromCanonicalName: "demo.Handle",
				ToSymbolID:        "sym-2",
				ToCanonicalName:   "demo.Child",
				Status:            facts.StepAccepted,
				Rationale:         "evidence-backed",
				Evidence: []facts.EvidenceRef{
					{
						SymbolID:  "sym-1",
						FilePath:  "pkg/file.go",
						StartLine: 3,
						EndLine:   5,
						Snippet:   "return child()",
						Source:    "tree-sitter-go",
					},
				},
			},
		},
		Ambiguous: []facts.ReviewStep{
			{
				ID:                "step-2",
				FromSymbolID:      "sym-1",
				FromCanonicalName: "demo.Handle",
				ToCanonicalName:   "demo.Unknown",
				Status:            facts.StepAmbiguous,
				Rationale:         "needs review",
			},
		},
		Rejected: []facts.ReviewStep{
			{
				ID:                "step-3",
				FromSymbolID:      "sym-1",
				FromCanonicalName: "demo.Handle",
				ToCanonicalName:   "demo.Test",
				Status:            facts.StepRejected,
				Rationale:         "test edge",
			},
		},
		UncertaintyNotes: []string{"cycle avoided", "max depth reached"},
	}

	if err := store.SaveReviewFlow(flow); err != nil {
		t.Fatalf("SaveReviewFlow: %v", err)
	}

	got, err := store.GetReviewFlow(flow.ID)
	if err != nil {
		t.Fatalf("GetReviewFlow: %v", err)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at mismatch: got %s want %s", got.CreatedAt, createdAt)
	}
	if len(got.Steps) != 1 || len(got.Steps[0].Evidence) != 1 {
		t.Fatalf("unexpected review flow round-trip: %+v", got)
	}
	if got.Steps[0].Evidence[0].Source != "tree-sitter-go" {
		t.Fatalf("evidence source mismatch: %+v", got.Steps[0].Evidence[0])
	}
	if got.Accepted[0].ToSymbolID != "sym-2" || got.Ambiguous[0].ToCanonicalName != "demo.Unknown" || got.Rejected[0].ToCanonicalName != "demo.Test" {
		t.Fatalf("unexpected review flow sections: %+v", got)
	}
	if len(got.UncertaintyNotes) != 2 || got.UncertaintyNotes[0] != "cycle avoided" {
		t.Fatalf("uncertainty notes mismatch: %+v", got.UncertaintyNotes)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "facts.sqlite")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
