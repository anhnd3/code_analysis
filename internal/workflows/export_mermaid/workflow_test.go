package export_mermaid

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/sequence"
)

func TestEnsureNonEmptyStageGuards(t *testing.T) {
	t.Run("roots", func(t *testing.T) {
		if err := ensureNonEmptyRoots(entrypoint.Result{}, RootFilterHTTP); err == nil {
			t.Fatal("expected empty-root guard to fail")
		}
	})

	t.Run("chains", func(t *testing.T) {
		if err := ensureNonEmptyChains(flow.Bundle{}); err == nil {
			t.Fatal("expected empty-chain guard to fail")
		}
	})

	t.Run("reduced", func(t *testing.T) {
		if err := ensureNonEmptyReducedChain(reduced.Chain{}); err == nil {
			t.Fatal("expected empty-reduced-chain guard to fail")
		}
	})

	t.Run("sequence", func(t *testing.T) {
		if err := ensureNonEmptySequence(sequence.Diagram{}); err == nil {
			t.Fatal("expected empty-sequence guard to fail")
		}
	})
}

func TestFilterRootsHonorsSelectorWithinType(t *testing.T) {
	full := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "root_health", CanonicalName: "GET /health", RootType: entrypoint.RootHTTP},
			{NodeID: "root_info", CanonicalName: "GET /info", RootType: entrypoint.RootHTTP},
			{NodeID: "bootstrap_start", CanonicalName: "cmd.Start", RootType: entrypoint.RootBootstrap},
		},
	}

	filtered := filterRoots(full, Request{
		RootType:     RootFilterHTTP,
		RootSelector: "GET /info",
	})
	if len(filtered.Roots) != 1 {
		t.Fatalf("expected one filtered root, got %+v", filtered.Roots)
	}
	if filtered.Roots[0].CanonicalName != "GET /info" {
		t.Fatalf("expected GET /info, got %+v", filtered.Roots[0])
	}

	filteredByNode := filterRoots(full, Request{
		RootType:     RootFilterHTTP,
		RootSelector: "root_health",
	})
	if len(filteredByNode.Roots) != 1 || filteredByNode.Roots[0].NodeID != "root_health" {
		t.Fatalf("expected node-id selector to narrow to root_health, got %+v", filteredByNode.Roots)
	}
}
