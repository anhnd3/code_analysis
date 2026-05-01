package goextractor

import (
	"path/filepath"
	"testing"

	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/tests/fixtures"
)

func semanticHintsFile(t *testing.T, name string) symbol.FileRef {
	t.Helper()
	workspace := fixtures.WorkspacePath(t, "semantic_hints")
	abs := filepath.Join(workspace, name)
	return symbol.FileRef{
		RepositoryID:   "semantic_hints",
		RepositoryRoot: workspace,
		AbsolutePath:   abs,
		RelativePath:   name,
		Language:       "go",
	}
}

func mustFindSymbol(t *testing.T, symbols []symbol.Symbol, canonical string) symbol.Symbol {
	t.Helper()
	for _, sym := range symbols {
		if sym.CanonicalName == canonical {
			return sym
		}
	}
	t.Fatalf("missing symbol %s in %+v", canonical, symbols)
	return symbol.Symbol{}
}

func mustFindHint(t *testing.T, hints []executionhint.Hint, kind executionhint.HintKind) executionhint.Hint {
	t.Helper()
	matches := []executionhint.Hint{}
	for _, hint := range hints {
		if hint.Kind == kind {
			matches = append(matches, hint)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 %s hint, got %d: %+v", kind, len(matches), hints)
	}
	return matches[0]
}

func hasHint(hints []executionhint.Hint, kind executionhint.HintKind) bool {
	for _, hint := range hints {
		if hint.Kind == kind {
			return true
		}
	}
	return false
}

func TestExtractorEmitsReturnHandlerHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "return_handler.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	source := mustFindSymbol(t, result.Symbols, "handler.ReturnHandlerFunc")
	target := mustFindSymbol(t, result.Symbols, "handler.ReturnHandlerFunc.$closure_return_0")
	hint := mustFindHint(t, result.Hints, executionhint.HintReturnHandler)

	if hint.SourceSymbolID != string(source.ID) {
		t.Fatalf("expected source %s, got %s", source.ID, hint.SourceSymbolID)
	}
	if hint.TargetSymbolID != string(target.ID) {
		t.Fatalf("expected target ID %s, got %s", target.ID, hint.TargetSymbolID)
	}
	if hint.TargetSymbol != target.CanonicalName {
		t.Fatalf("expected target symbol %s, got %s", target.CanonicalName, hint.TargetSymbol)
	}
	if hint.Evidence == "" {
		t.Fatal("expected non-empty evidence")
	}
	if hint.OrderIndex != 0 {
		t.Fatalf("expected order_index 0, got %d", hint.OrderIndex)
	}
}

func TestExtractorEmitsInlineSpawnHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "goroutine.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	source := mustFindSymbol(t, result.Symbols, "handler.SpawnWorker")
	target := mustFindSymbol(t, result.Symbols, "handler.SpawnWorker.$inline_handler_0")
	hint := mustFindHint(t, result.Hints, executionhint.HintSpawn)

	if hint.SourceSymbolID != string(source.ID) {
		t.Fatalf("expected source %s, got %s", source.ID, hint.SourceSymbolID)
	}
	if hint.TargetSymbolID != string(target.ID) {
		t.Fatalf("expected target ID %s, got %s", target.ID, hint.TargetSymbolID)
	}
	if hint.TargetSymbol != target.CanonicalName {
		t.Fatalf("expected target symbol %s, got %s", target.CanonicalName, hint.TargetSymbol)
	}
	if hint.Evidence == "" {
		t.Fatal("expected non-empty evidence")
	}
	if hint.OrderIndex != 0 {
		t.Fatalf("expected order_index 0, got %d", hint.OrderIndex)
	}
}

func TestExtractorEmitsDirectSpawnHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "spawn_symbol.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	source := mustFindSymbol(t, result.Symbols, "handler.SpawnNamed")
	hint := mustFindHint(t, result.Hints, executionhint.HintSpawn)

	if hint.SourceSymbolID != string(source.ID) {
		t.Fatalf("expected source %s, got %s", source.ID, hint.SourceSymbolID)
	}
	if hint.TargetSymbolID != "" {
		t.Fatalf("expected empty direct-call target ID, got %s", hint.TargetSymbolID)
	}
	if hint.TargetSymbol != "handler.runJob" {
		t.Fatalf("expected target symbol handler.runJob, got %s", hint.TargetSymbol)
	}
	if hint.Evidence == "" {
		t.Fatal("expected non-empty evidence")
	}
	if hint.OrderIndex != 0 {
		t.Fatalf("expected order_index 0, got %d", hint.OrderIndex)
	}
}

func TestExtractorEmitsSelectorDeferHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "defer.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	source := mustFindSymbol(t, result.Symbols, "handler.DeferCleanup")
	hint := mustFindHint(t, result.Hints, executionhint.HintDefer)

	if hint.SourceSymbolID != string(source.ID) {
		t.Fatalf("expected source %s, got %s", source.ID, hint.SourceSymbolID)
	}
	if hint.TargetSymbolID != "" {
		t.Fatalf("expected empty selector target ID, got %s", hint.TargetSymbolID)
	}
	if hint.TargetSymbol != "fmt.Println" {
		t.Fatalf("expected target symbol fmt.Println, got %s", hint.TargetSymbol)
	}
	if hint.Evidence == "" {
		t.Fatal("expected non-empty evidence")
	}
	if hint.OrderIndex != 0 {
		t.Fatalf("expected order_index 0, got %d", hint.OrderIndex)
	}
}

func TestExtractorEmitsInlineDeferHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "defer_inline.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	source := mustFindSymbol(t, result.Symbols, "handler.DeferInline")
	target := mustFindSymbol(t, result.Symbols, "handler.DeferInline.$inline_handler_0")
	hint := mustFindHint(t, result.Hints, executionhint.HintDefer)

	if hint.SourceSymbolID != string(source.ID) {
		t.Fatalf("expected source %s, got %s", source.ID, hint.SourceSymbolID)
	}
	if hint.TargetSymbolID != string(target.ID) {
		t.Fatalf("expected target ID %s, got %s", target.ID, hint.TargetSymbolID)
	}
	if hint.TargetSymbol != target.CanonicalName {
		t.Fatalf("expected target symbol %s, got %s", target.CanonicalName, hint.TargetSymbol)
	}
	if hint.Evidence == "" {
		t.Fatal("expected non-empty evidence")
	}
	if hint.OrderIndex != 0 {
		t.Fatalf("expected order_index 0, got %d", hint.OrderIndex)
	}
}

func TestExtractorEmitsWaitHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "waitgroup.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	source := mustFindSymbol(t, result.Symbols, "handler.WaitGroupJoin")
	hint := mustFindHint(t, result.Hints, executionhint.HintWait)

	if hint.SourceSymbolID != string(source.ID) {
		t.Fatalf("expected source %s, got %s", source.ID, hint.SourceSymbolID)
	}
	if hint.TargetSymbolID != string(source.ID) {
		t.Fatalf("expected self-target ID %s, got %s", source.ID, hint.TargetSymbolID)
	}
	if hint.TargetSymbol != source.CanonicalName {
		t.Fatalf("expected self-target symbol %s, got %s", source.CanonicalName, hint.TargetSymbol)
	}
	if hint.Evidence == "" {
		t.Fatal("expected non-empty evidence")
	}
	if hint.OrderIndex != 2 {
		t.Fatalf("expected order_index 2, got %d", hint.OrderIndex)
	}
	if hasHint(result.Hints, executionhint.HintDefer) {
		t.Fatalf("did not expect nested defer inside goroutine to leak into parent hints: %+v", result.Hints)
	}
}

func TestExtractorEmitsBranchHint(t *testing.T) {
	e := New()
	result, err := e.ExtractFile(semanticHintsFile(t, "branch.go"))
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if !hasHint(result.Hints, executionhint.HintBranch) {
		t.Fatalf("expected HintBranch in hints; got %+v", result.Hints)
	}
}
