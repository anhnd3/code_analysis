package graph_build

import (
	"strings"
	"testing"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
)

func buildSemanticSnapshot(t *testing.T, snapshotID string, hints []executionhint.Hint, extraSymbols []symbol.Symbol) graph.GraphSnapshot {
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

	allSymbols := append([]symbol.Symbol{{
		ID:            "sym_src",
		RepositoryID:  "repo_sem",
		FilePath:      "handler.go",
		PackageName:   "handler",
		Name:          "Handle",
		CanonicalName: "handler.Handle",
		Kind:          symbol.KindFunction,
	}}, extraSymbols...)

	extraction := symbol_index.Result{
		Repositories: []symbol_index.RepositoryExtraction{{
			Repository: inventory.Repositories[0],
			Files: []symbol.FileExtractionResult{{
				FilePath:    "handler.go",
				Language:    "go",
				PackageName: "handler",
				Symbols:     allSymbols,
				Hints:       hints,
			}},
		}},
	}
	return builder.Build("ws_sem", snapshotID, inventory, extraction, nil).Snapshot
}

func nodeIDsByCanonical(nodes []graph.Node) map[string]string {
	ids := map[string]string{}
	for _, node := range nodes {
		ids[node.CanonicalName] = node.ID
	}
	return ids
}

func edgesByKind(edges []graph.Edge, kind graph.EdgeKind) []graph.Edge {
	result := []graph.Edge{}
	for _, edge := range edges {
		if edge.Kind == kind {
			result = append(result, edge)
		}
	}
	return result
}

func TestGraphBuildPersistsReturnsHandlerEdge(t *testing.T) {
	closureSym := symbol.Symbol{
		ID:            "sym_closure",
		RepositoryID:  "repo_sem",
		FilePath:      "handler.go",
		PackageName:   "handler",
		Name:          "$closure_return_0",
		CanonicalName: "handler.Handle.$closure_return_0",
		Kind:          symbol.KindFunction,
		Properties: map[string]string{
			"synthetic": "true",
		},
	}
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbolID: "sym_closure",
		TargetSymbol:   "handler.Handle.$closure_return_0",
		Kind:           executionhint.HintReturnHandler,
		Evidence:       "return func(input string) string { return input }",
		OrderIndex:     0,
	}}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, []symbol.Symbol{closureSym})
	edges := edgesByKind(snap.Edges, graph.EdgeReturnsHandler)
	if len(edges) != 1 {
		t.Fatalf("expected 1 RETURNS_HANDLER edge, got %d: %+v", len(edges), snap.Edges)
	}
	edge := edges[0]
	nodeIDs := nodeIDsByCanonical(snap.Nodes)
	if edge.From != nodeIDs["handler.Handle"] {
		t.Fatalf("expected source node %s, got %s", nodeIDs["handler.Handle"], edge.From)
	}
	if edge.To != nodeIDs["handler.Handle.$closure_return_0"] {
		t.Fatalf("expected target node %s, got %s", nodeIDs["handler.Handle.$closure_return_0"], edge.To)
	}
	if edge.Evidence.Source == "" {
		t.Fatal("expected evidence to be populated")
	}
	if edge.Properties["order_index"] != "0" {
		t.Fatalf("expected order_index 0, got %v", edge.Properties)
	}
	if edge.Properties["synthetic_target"] != "true" {
		t.Fatalf("expected synthetic_target=true, got %v", edge.Properties)
	}
}

func TestGraphBuildPersistsSpawnsEdgeWithExactCanonicalFallback(t *testing.T) {
	workerSym := symbol.Symbol{
		ID:            "sym_worker",
		RepositoryID:  "repo_sem",
		FilePath:      "handler.go",
		PackageName:   "handler",
		Name:          "Run",
		CanonicalName: "handler.Run",
		Kind:          symbol.KindFunction,
	}
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "handler.Run",
		Kind:           executionhint.HintSpawn,
		Evidence:       "go Run()",
		OrderIndex:     1,
	}}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, []symbol.Symbol{workerSym})
	edges := edgesByKind(snap.Edges, graph.EdgeSpawns)
	if len(edges) != 1 {
		t.Fatalf("expected 1 SPAWNS edge, got %d: %+v", len(edges), snap.Edges)
	}
	edge := edges[0]
	nodeIDs := nodeIDsByCanonical(snap.Nodes)
	if edge.To != nodeIDs["handler.Run"] {
		t.Fatalf("expected target node %s, got %s", nodeIDs["handler.Run"], edge.To)
	}
	if edge.Confidence.Tier != graph.ConfidenceInferred {
		t.Fatalf("expected inferred confidence for canonical fallback, got %+v", edge.Confidence)
	}
	if strings.HasPrefix(edge.To, "unresolved_") {
		t.Fatalf("did not expect unresolved target: %+v", edge)
	}
}

func TestGraphBuildPersistsDeferEdgeToSyntheticTarget(t *testing.T) {
	inlineSym := symbol.Symbol{
		ID:            "sym_inline",
		RepositoryID:  "repo_sem",
		FilePath:      "handler.go",
		PackageName:   "handler",
		Name:          "$inline_handler_0",
		CanonicalName: "handler.Handle.$inline_handler_0",
		Kind:          symbol.KindFunction,
		Properties: map[string]string{
			"synthetic": "true",
		},
	}
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbolID: "sym_inline",
		TargetSymbol:   "handler.Handle.$inline_handler_0",
		Kind:           executionhint.HintDefer,
		Evidence:       "defer func() { cleanup() }()",
		OrderIndex:     0,
	}}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, []symbol.Symbol{inlineSym})
	edges := edgesByKind(snap.Edges, graph.EdgeDefers)
	if len(edges) != 1 {
		t.Fatalf("expected 1 DEFERS edge, got %d: %+v", len(edges), snap.Edges)
	}
	nodeIDs := nodeIDsByCanonical(snap.Nodes)
	if edges[0].To != nodeIDs["handler.Handle.$inline_handler_0"] {
		t.Fatalf("expected synthetic defer target %s, got %s", nodeIDs["handler.Handle.$inline_handler_0"], edges[0].To)
	}
}

func TestGraphBuildPersistsWaitsOnEdgeAsStableSelfTarget(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbolID: "sym_src",
		TargetSymbol:   "handler.Handle",
		Kind:           executionhint.HintWait,
		Evidence:       "wg.Wait()",
		OrderIndex:     2,
	}}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, nil)
	edges := edgesByKind(snap.Edges, graph.EdgeWaitsOn)
	if len(edges) != 1 {
		t.Fatalf("expected 1 WAITS_ON edge, got %d: %+v", len(edges), snap.Edges)
	}
	edge := edges[0]
	nodeIDs := nodeIDsByCanonical(snap.Nodes)
	if edge.From != nodeIDs["handler.Handle"] || edge.To != nodeIDs["handler.Handle"] {
		t.Fatalf("expected stable self-target wait edge, got %+v", edge)
	}
}

func TestGraphBuildSkipsUnresolvedSemanticHints(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbol:   "handler.Missing",
		Kind:           executionhint.HintSpawn,
		Evidence:       "go Missing()",
		OrderIndex:     0,
	}}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, nil)
	if got := len(edgesByKind(snap.Edges, graph.EdgeSpawns)); got != 0 {
		t.Fatalf("expected unresolved semantic hint to be skipped, got %d edges: %+v", got, snap.Edges)
	}
}

func TestGraphBuildDedupesDuplicateSemanticHints(t *testing.T) {
	hints := []executionhint.Hint{
		{
			SourceSymbolID: "sym_src",
			TargetSymbolID: "sym_src",
			TargetSymbol:   "handler.Handle",
			Kind:           executionhint.HintWait,
			Evidence:       "wg.Wait()",
			OrderIndex:     2,
		},
		{
			SourceSymbolID: "sym_src",
			TargetSymbolID: "sym_src",
			TargetSymbol:   "handler.Handle",
			Kind:           executionhint.HintWait,
			Evidence:       "wg.Wait()",
			OrderIndex:     2,
		},
	}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, nil)
	if got := len(edgesByKind(snap.Edges, graph.EdgeWaitsOn)); got != 1 {
		t.Fatalf("expected duplicate semantic hints to dedupe to 1 edge, got %d: %+v", got, snap.Edges)
	}
}

func TestGraphBuildPersistsBranchHintAsBranchEdge(t *testing.T) {
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbolID: "sym_src",
		TargetSymbol:   "handler.Handle",
		Kind:           executionhint.HintBranch,
		Evidence:       "if enabled",
		OrderIndex:     0,
	}}

	snap := buildSemanticSnapshot(t, "snap_sem_a", hints, nil)
	if got := len(edgesByKind(snap.Edges, graph.EdgeBranches)); got != 1 {
		t.Fatalf("expected BRANCHES edge for branch hint, got %d: %+v", got, snap.Edges)
	}
}

func TestGraphBuildSemanticEdgeIDsRemainStableAcrossSnapshots(t *testing.T) {
	closureSym := symbol.Symbol{
		ID:            "sym_closure",
		RepositoryID:  "repo_sem",
		FilePath:      "handler.go",
		PackageName:   "handler",
		Name:          "$closure_return_0",
		CanonicalName: "handler.Handle.$closure_return_0",
		Kind:          symbol.KindFunction,
		Properties: map[string]string{
			"synthetic": "true",
		},
	}
	hints := []executionhint.Hint{{
		SourceSymbolID: "sym_src",
		TargetSymbolID: "sym_closure",
		TargetSymbol:   "handler.Handle.$closure_return_0",
		Kind:           executionhint.HintReturnHandler,
		Evidence:       "return func() {}",
		OrderIndex:     0,
	}}

	first := buildSemanticSnapshot(t, "snap_sem_a", hints, []symbol.Symbol{closureSym})
	second := buildSemanticSnapshot(t, "snap_sem_b", hints, []symbol.Symbol{closureSym})

	firstEdges := edgesByKind(first.Edges, graph.EdgeReturnsHandler)
	secondEdges := edgesByKind(second.Edges, graph.EdgeReturnsHandler)
	if len(firstEdges) != 1 || len(secondEdges) != 1 {
		t.Fatalf("expected 1 semantic edge in each snapshot, got %d and %d", len(firstEdges), len(secondEdges))
	}
	if firstEdges[0].ID != secondEdges[0].ID {
		t.Fatalf("expected stable semantic edge IDs, got %s and %s", firstEdges[0].ID, secondEdges[0].ID)
	}
	if firstEdges[0].From != secondEdges[0].From || firstEdges[0].To != secondEdges[0].To {
		t.Fatalf("expected stable semantic node targets, got %+v and %+v", firstEdges[0], secondEdges[0])
	}
}
