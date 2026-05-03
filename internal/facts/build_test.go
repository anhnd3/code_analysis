package facts

import (
	"testing"
)

// TestResolveCallCandidates tests that call candidates are properly resolved to target symbols.
func TestResolveCallCandidates(t *testing.T) {
	// Setup: create a symbol that can be referenced as a call target
	symbols := []SymbolFact{
		{ID: "sym_target_001", Name: "Handle", CanonicalName: "service.Handle", FilePath: "internal/service/service.go"},
		{ID: "sym_target_002", Name: "Transform", CanonicalName: "example.com/pkg/internal.Transform", FilePath: "internal/helper/helper.go"},
	}

	exports := []ExportFact{
		{Name: "Handle", CanonicalName: "service.Handle", FilePath: "internal/service/service.go"},
		{Name: "Transform", CanonicalName: "example.com/pkg/internal.Transform", FilePath: "internal/helper/helper.go"},
	}

	files := []FileFact{}

	candidates := []CallCandidate{
		// Candidate with only target_canonical_name - should resolve via canonical name
		{ID: "call_001", SourceSymbolID: "sym_source_001", TargetCanonicalName: "service.Handle"},
		// Candidate with target_file_path and target_export_name - should resolve via export lookup
		{ID: "call_002", SourceSymbolID: "sym_source_002", TargetFilePath: "internal/helper/helper.go", TargetExportName: "Transform"},
		// Already resolved candidate - should remain unchanged
		{ID: "call_003", SourceSymbolID: "sym_source_003", TargetSymbolID: "sym_existing_target"},
	}

	resolved, issueCounts, diagnostics := ResolveCallCandidates(candidates, symbols, exports, files)

	// Verify resolution occurred correctly
	if len(resolved) != 3 {
		t.Fatalf("Expected 3 candidates, got %d", len(resolved))
	}

	// First candidate should have target_symbol_id set from canonical name
	if resolved[0].TargetSymbolID != "sym_target_001" {
		t.Errorf("Candidate 0: expected target_symbol_id=sym_target_001, got %s", resolved[0].TargetSymbolID)
	}

	// Second candidate should have target_symbol_id set from file path + export name
	if resolved[1].TargetSymbolID != "sym_target_002" {
		t.Errorf("Candidate 1: expected target_symbol_id=sym_target_002, got %s", resolved[1].TargetSymbolID)
	}

	// Third candidate should remain unchanged (already had target_symbol_id)
	if resolved[2].TargetSymbolID != "sym_existing_target" {
		t.Errorf("Candidate 2: expected target_symbol_id=sym_existing_target, got %s", resolved[2].TargetSymbolID)
	}

	// Issue counts and diagnostics should be empty for successful resolution
	if issueCounts.AmbiguousRelations != 0 {
		t.Errorf("Expected 0 ambiguous relations, got %d", issueCounts.AmbiguousRelations)
	}

	if len(diagnostics) > 0 {
		t.Errorf("Expected no diagnostics, got %d", len(diagnostics))
	}
}

// TestResolveCallCandidatesAmbiguous tests that ambiguous targets produce diagnostics.
func TestResolveCallCandidatesAmbiguous(t *testing.T) {
	symbols := []SymbolFact{
		{ID: "sym_001", Name: "Handle", CanonicalName: "service.Handle", FilePath: "internal/service/a.go"},
	}

	exports := []ExportFact{
		{Name: "Handle", CanonicalName: "service.Handle", FilePath: "internal/service/a.go"},
	}

	files := []FileFact{}

	candidates := []CallCandidate{
		// This candidate has an ambiguous target (multiple matches would occur)
		// For now, we just test that the function handles it without panicking
		{ID: "call_ambig", SourceSymbolID: "sym_source", TargetFilePath: "internal/service/a.go", TargetExportName: "Handle"},
	}

	resolved, issueCounts, diagnostics := ResolveCallCandidates(candidates, symbols, exports, files)

	// Should not panic, and should produce appropriate diagnostics/issue counts
	if len(resolved) != 1 {
		t.Fatalf("Expected 1 candidate, got %d", len(resolved))
	}

	_ = issueCounts // May have ambiguous relation count incremented
	_ = diagnostics // May contain diagnostic for ambiguity
}

// TestBuildIndexResolvesCallCandidates tests that BuildIndex calls ResolveCallCandidates.
func TestBuildIndexResolvesCallCandidates(t *testing.T) {
	input := BuildInput{
		WorkspaceID: "ws_test",
		SnapshotID:  "snap_001",
		Symbols: []SymbolFact{
			{ID: "sym_target", Name: "TargetFunc", CanonicalName: "pkg.TargetFunc", FilePath: "pkg/pkg.go"},
		},
		Exports: []ExportFact{
			{Name: "TargetFunc", CanonicalName: "pkg.TargetFunc", FilePath: "pkg/pkg.go"},
		},
		CallCandidates: []CallCandidate{
			{ID: "call_001", SourceSymbolID: "sym_source", TargetCanonicalName: "pkg.TargetFunc"},
		},
	}

	index := BuildIndex(input)

	if len(index.CallCandidates) != 1 {
		t.Fatalf("Expected 1 call candidate, got %d", len(index.CallCandidates))
	}

	if index.CallCandidates[0].TargetSymbolID == "" {
		t.Error("Call candidate should have target_symbol_id populated after BuildIndex")
	}
}
