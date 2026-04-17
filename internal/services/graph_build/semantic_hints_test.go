package graph_build

import (
	"testing"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
)

// buildSemanticSnapshot is a test helper that produces a graph snapshot for a
// single source symbol that carries the supplied hints, alongside a target
// closure symbol for resolution tests.
func buildSemanticSnapshot(t *testing.T, hints []executionhint.Hint, extraSymbols []symbol.Symbol) graph.GraphSnapshot {
	t.Helper()
	builder := New(progress.NoopReporter{})
	inventory := repository.Inventory{
		WorkspaceID: "ws_sem",
		Repositories: []repository.Manifest{{
			ID:                "repo_sem",
			Name:              "sem",
			RootPath:          "/sem",
			Role:              repository.RoleService,
			CandidateServices: []service.Manifest{{ID: "svc_sem", Name: "sem"}},
		}},
	}

	allSymbols := append([]symbol.Symbol{
		{
			ID:            "sym_src",
			RepositoryID:  "repo_sem",
			FilePath:      "handler.go",
			PackageName:   "handler",
			Name:          "Handle",
			CanonicalName: "handler.Handle",
			Kind:          symbol.KindFunction,
		},
	}, extraSymbols...)

	extraction := symbol_index.Result{
		Repositories: []symbol_index.RepositoryExtraction{{
			Repository: inventory.Repositories[0],
			Files: []symbol.FileExtractionResult{{
				FilePath:    "handler.go",
				PackageName: "handler",
				Symbols:     allSymbols,
				Hints:       hints,
			}},
		}},
	}
	return builder.Build("ws_sem", "snap_sem", inventory, extraction, nil).Snapshot
}

// edgeKindPresent tests whether the given edge kind appears anywhere in edges.
func edgeKindPresent(edges []graph.Edge, k graph.EdgeKind) bool {
	for _, e := range edges {
		if e.Kind == k {
			return true
		}
	}
	return false
}

// ── RETURNS_HANDLER edge ──────────────────────────────────────────────────────

func TestGraphBuildPersistsReturnsHandlerEdge(t *testing.T) {
	closureSym := symbol.Symbol{
		ID:            "sym_closure",
		RepositoryID:  "repo_sem",
		FilePath:      "handler.go",
		PackageName:   "handler",
		Name:          "Handle.$closure_return_0",
		CanonicalName: "handler.Handle.$closure_return_0",
		Kind:          symbol.KindFunction,
	}
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "handler.Handle.$closure_return_0",
		Kind:           executionhint.HintReturnHandler,
		Evidence:       "return func_literal at line 5",
	}}
	snap := buildSemanticSnapshot(t, hints, []symbol.Symbol{closureSym})
	if !edgeKindPresent(snap.Edges, graph.EdgeReturnsHandler) {
		t.Fatalf("expected RETURNS_HANDLER edge; edges: %+v", snap.Edges)
	}
}

// ── SPAWNS edge ───────────────────────────────────────────────────────────────

func TestGraphBuildPersistsSpawnsEdge(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "",
		Kind:           executionhint.HintSpawn,
		Evidence:       "go func_literal",
	}}
	snap := buildSemanticSnapshot(t, hints, nil)
	if !edgeKindPresent(snap.Edges, graph.EdgeSpawns) {
		t.Fatalf("expected SPAWNS edge; edges: %+v", snap.Edges)
	}
}

// ── DEFERS edge ───────────────────────────────────────────────────────────────

func TestGraphBuildPersistsDeferEdge(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "",
		Kind:           executionhint.HintDefer,
		Evidence:       "defer fmt.Println",
	}}
	snap := buildSemanticSnapshot(t, hints, nil)
	if !edgeKindPresent(snap.Edges, graph.EdgeDefers) {
		t.Fatalf("expected DEFERS edge; edges: %+v", snap.Edges)
	}
}

// ── WAITS_ON edge ─────────────────────────────────────────────────────────────

func TestGraphBuildPersistsWaitsOnEdge(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "",
		Kind:           executionhint.HintWait,
		Evidence:       "wg.Wait() at line 14",
	}}
	snap := buildSemanticSnapshot(t, hints, nil)
	if !edgeKindPresent(snap.Edges, graph.EdgeWaitsOn) {
		t.Fatalf("expected WAITS_ON edge; edges: %+v", snap.Edges)
	}
}

// ── Branch → CALLS edge ───────────────────────────────────────────────────────

func TestGraphBuildPersistsBranchHintAsCallsEdge(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "",
		Kind:           executionhint.HintBranch,
		Evidence:       "if p.kind == \"json\"",
	}}
	snap := buildSemanticSnapshot(t, hints, nil)
	if !edgeKindPresent(snap.Edges, graph.EdgeCalls) {
		t.Fatalf("expected CALLS edge for branch hint; edges: %+v", snap.Edges)
	}
}
