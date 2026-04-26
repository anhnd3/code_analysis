package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analysis-module/internal/facts"
	factsqlite "analysis-module/internal/store/sqlite"
)

func TestInspectFunctionBuildsPacketAndSlicesContext(t *testing.T) {
	svc, fixture := newQueryFixture(t)

	packet, err := svc.InspectFunction(InspectRequest{
		WorkspaceID:   fixture.workspaceID,
		SnapshotID:    fixture.snapshotID,
		Symbol:        "demo.Handle",
		ContextWindow: 2,
	})
	if err != nil {
		t.Fatalf("InspectFunction: %v", err)
	}

	if packet.RootSymbol.CanonicalName != "demo.Handle" {
		t.Fatalf("unexpected root symbol: %+v", packet.RootSymbol)
	}
	if !strings.Contains(packet.FunctionSource, "func Handle() string {") || !strings.Contains(packet.FunctionSource, "return helper()") {
		t.Fatalf("function source was not sliced correctly: %q", packet.FunctionSource)
	}
	if !strings.Contains(packet.SurroundingContext, "package demo") || !strings.Contains(packet.SurroundingContext, "func helper() string {") {
		t.Fatalf("surrounding context missing expected lines: %q", packet.SurroundingContext)
	}
	if len(packet.NearbyTests) != 1 || packet.NearbyTests[0].CanonicalName != "demo.TestHandle" {
		t.Fatalf("unexpected nearby tests: %+v", packet.NearbyTests)
	}
	if len(packet.OutgoingCandidates) != 2 {
		t.Fatalf("unexpected candidate count: %+v", packet.OutgoingCandidates)
	}
	if packet.OutgoingCandidates[0].TargetSymbolID != fixture.helperID || packet.OutgoingCandidates[1].TargetSymbolID != fixture.testID {
		t.Fatalf("candidate ordering mismatch: %+v", packet.OutgoingCandidates)
	}
}

func TestResolveSymbolMatchesCanonicalAndID(t *testing.T) {
	svc, fixture := newQueryFixture(t)

	resolved, err := svc.ResolveSymbol(fixture.workspaceID, fixture.snapshotID, "demo.Handle")
	if err != nil {
		t.Fatalf("ResolveSymbol canonical: %v", err)
	}
	if resolved.ID != fixture.rootID {
		t.Fatalf("expected %s, got %+v", fixture.rootID, resolved)
	}

	resolvedByID, err := svc.ResolveSymbol(fixture.workspaceID, fixture.snapshotID, fixture.helperID)
	if err != nil {
		t.Fatalf("ResolveSymbol id: %v", err)
	}
	if resolvedByID.CanonicalName != "demo.helper" {
		t.Fatalf("expected helper symbol, got %+v", resolvedByID)
	}

	byID, err := svc.SymbolByID(fixture.workspaceID, fixture.snapshotID, fixture.testID)
	if err != nil {
		t.Fatalf("SymbolByID: %v", err)
	}
	if byID.CanonicalName != "demo.TestHandle" {
		t.Fatalf("expected test symbol, got %+v", byID)
	}
}

type queryFixture struct {
	workspaceID string
	snapshotID  string
	rootID      string
	helperID    string
	testID      string
}

func newQueryFixture(t *testing.T) (Service, queryFixture) {
	t.Helper()

	rootDir := t.TempDir()
	repoRoot := filepath.Join(rootDir, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	filePath := filepath.Join(repoRoot, "flow.go")
	content := strings.Join([]string{
		"package demo",
		"",
		"func Handle() string {",
		"    return helper()",
		"}",
		"",
		"func helper() string {",
		"    return \"ok\"",
		"}",
		"",
		"func TestHandle() {}",
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := factsqlite.New(factsqlite.PathFor(rootDir, "ws-1", "snap-1"))
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	index := facts.Index{
		WorkspaceID: "ws-1",
		SnapshotID:  "snap-1",
		Files: []facts.FileFact{
			{
				ID:             "file-1",
				RepositoryID:   "repo-1",
				RepositoryRoot: repoRoot,
				RelativePath:   "flow.go",
				AbsolutePath:   filePath,
				Language:       "go",
				PackageName:    "demo",
			},
		},
		Symbols: []facts.SymbolFact{
			{
				ID:            "sym-root",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.Handle",
				Name:          "Handle",
				Kind:          "function",
				StartLine:     3,
				EndLine:       5,
			},
			{
				ID:            "sym-helper",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.helper",
				Name:          "helper",
				Kind:          "function",
				StartLine:     7,
				EndLine:       9,
			},
			{
				ID:            "sym-test",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.TestHandle",
				Name:          "TestHandle",
				Kind:          "test_function",
				StartLine:     11,
				EndLine:       11,
			},
		},
		CallCandidates: []facts.CallCandidate{
			{
				ID:                  "call-helper",
				SourceSymbolID:      "sym-root",
				TargetSymbolID:      "sym-helper",
				TargetCanonicalName: "demo.helper",
				Relationship:        "calls",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.9,
				OrderIndex:          1,
			},
			{
				ID:                  "call-test",
				SourceSymbolID:      "sym-root",
				TargetSymbolID:      "sym-test",
				TargetCanonicalName: "demo.TestHandle",
				Relationship:        "test",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.8,
				OrderIndex:          2,
			},
		},
		Tests: []facts.TestFact{
			{
				ID:            "test-1",
				SymbolID:      "sym-test",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.TestHandle",
				Name:          "TestHandle",
				StartLine:     11,
				EndLine:       11,
			},
		},
	}

	if err := store.SaveIndex(index); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	svc := New(rootDir)
	return svc, queryFixture{
		workspaceID: "ws-1",
		snapshotID:  "snap-1",
		rootID:      "sym-root",
		helperID:    "sym-helper",
		testID:      "sym-test",
	}
}
