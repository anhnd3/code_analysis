package goextractor

import (
	"path/filepath"
	"testing"

	"analysis-module/internal/domain/executionhint"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/tests/fixtures"
)

// workspaceFile returns a FileRef pointing at a file inside the semantic_hints
// test fixture.
func semanticHintsFile(t *testing.T, name string) (symbol.FileRef, string) {
	t.Helper()
	workspace := fixtures.WorkspacePath(t, "semantic_hints")
	abs := filepath.Join(workspace, name)
	return symbol.FileRef{
		RepositoryID:   "semantic_hints",
		RepositoryRoot: workspace,
		AbsolutePath:   abs,
		RelativePath:   name,
		Language:       "go",
	}, workspace
}

// hasHint reports whether hints contains at least one entry matching kind.
func hasHint(hints []executionhint.Hint, kind executionhint.HintKind) bool {
	for _, h := range hints {
		if h.Kind == kind {
			return true
		}
	}
	return false
}

// ── Return-handler ────────────────────────────────────────────────────────────

func TestExtractorEmitsReturnHandlerHint(t *testing.T) {
	e := New()
	ref, _ := semanticHintsFile(t, "return_handler.go")
	result, err := e.ExtractFile(ref)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if !hasHint(result.Hints, executionhint.HintReturnHandler) {
		t.Fatalf("expected HintReturnHandler in hints; got %+v", result.Hints)
	}
}

// ── Goroutine spawn ───────────────────────────────────────────────────────────

func TestExtractorEmitsSpawnHint(t *testing.T) {
	e := New()
	ref, _ := semanticHintsFile(t, "goroutine.go")
	result, err := e.ExtractFile(ref)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if !hasHint(result.Hints, executionhint.HintSpawn) {
		t.Fatalf("expected HintSpawn in hints; got %+v", result.Hints)
	}
}

// ── Defer ─────────────────────────────────────────────────────────────────────

func TestExtractorEmitsDeferHint(t *testing.T) {
	e := New()
	ref, _ := semanticHintsFile(t, "defer.go")
	result, err := e.ExtractFile(ref)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if !hasHint(result.Hints, executionhint.HintDefer) {
		t.Fatalf("expected HintDefer in hints; got %+v", result.Hints)
	}
}

// ── WaitGroup.Wait ────────────────────────────────────────────────────────────

func TestExtractorEmitsWaitHint(t *testing.T) {
	e := New()
	ref, _ := semanticHintsFile(t, "waitgroup.go")
	result, err := e.ExtractFile(ref)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if !hasHint(result.Hints, executionhint.HintWait) {
		t.Fatalf("expected HintWait in hints; got %+v", result.Hints)
	}
}

// ── Branch ────────────────────────────────────────────────────────────────────

func TestExtractorEmitsBranchHint(t *testing.T) {
	e := New()
	ref, _ := semanticHintsFile(t, "branch.go")
	result, err := e.ExtractFile(ref)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}
	if !hasHint(result.Hints, executionhint.HintBranch) {
		t.Fatalf("expected HintBranch in hints; got %+v", result.Hints)
	}
}
