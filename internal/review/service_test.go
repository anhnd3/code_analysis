package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analysis-module/internal/facts"
	"analysis-module/internal/llm"
	factquery "analysis-module/internal/query"
	factsqlite "analysis-module/internal/store/sqlite"
)

func TestRunClassifiesAcceptedAmbiguousRejected(t *testing.T) {
	fixture := newReviewFixture(t)
	client := reviewClientFunc(func(req llm.ReviewRequest) (llm.ReviewResponse, error) {
		if req.RootSymbol.CanonicalName != "demo.Root" {
			return llm.ReviewResponse{}, nil
		}
		decisions := make([]llm.HopDecision, 0, len(req.Packet.OutgoingCandidates))
		for _, candidate := range req.Packet.OutgoingCandidates {
			decision := llm.HopDecision{
				TargetSymbolID:      candidate.TargetSymbolID,
				TargetCanonicalName: candidate.TargetCanonicalName,
				Rationale:           "stub decision",
			}
			switch candidate.TargetCanonicalName {
			case "demo.Child":
				decision.Status = facts.StepAccepted
			case "demo.Unresolved":
				decision.Status = facts.StepAccepted
			case "demo.Rejected":
				decision.Status = facts.StepRejected
			default:
				decision.Status = facts.StepAmbiguous
			}
			decisions = append(decisions, decision)
		}
		return llm.ReviewResponse{Decisions: decisions}, nil
	})

	result := runReviewFixture(t, fixture, client, Request{
		WorkspaceID: fixture.workspaceID,
		SnapshotID:  fixture.snapshotID,
		Symbol:      "demo.Root",
		MaxDepth:    0,
		MaxSteps:    10,
		OutDir:      fixture.reviewDir,
	})

	if len(result.Flow.Accepted) != 1 {
		t.Fatalf("expected exactly one accepted step, got %+v", result.Flow.Accepted)
	}
	if len(result.Flow.Ambiguous) != 1 {
		t.Fatalf("expected unresolved step to remain ambiguous, got %+v", result.Flow.Ambiguous)
	}
	if len(result.Flow.Rejected) != 1 {
		t.Fatalf("expected exactly one rejected step, got %+v", result.Flow.Rejected)
	}
	if result.Flow.Accepted[0].ToCanonicalName != "demo.Child" {
		t.Fatalf("expected resolved child to remain accepted, got %+v", result.Flow.Accepted[0])
	}
	if result.Flow.Ambiguous[0].ToCanonicalName != "demo.Unresolved" {
		t.Fatalf("expected unresolved candidate to stay ambiguous, got %+v", result.Flow.Ambiguous[0])
	}
}

func TestRunStopsAtCyclesWithoutLooping(t *testing.T) {
	fixture := newReviewFixture(t)
	client := reviewClientFunc(func(req llm.ReviewRequest) (llm.ReviewResponse, error) {
		switch req.RootSymbol.CanonicalName {
		case "demo.Root":
			return llm.ReviewResponse{
				Decisions: []llm.HopDecision{
					{
						TargetSymbolID:      "sym-child",
						TargetCanonicalName: "demo.Child",
						Status:              facts.StepAccepted,
						Rationale:           "follow child",
					},
				},
			}, nil
		case "demo.Child":
			return llm.ReviewResponse{
				Decisions: []llm.HopDecision{
					{
						TargetSymbolID:      "sym-root",
						TargetCanonicalName: "demo.Root",
						Status:              facts.StepAccepted,
						Rationale:           "cycle edge",
					},
				},
			}, nil
		default:
			return llm.ReviewResponse{}, fmt.Errorf("unexpected symbol %s", req.RootSymbol.CanonicalName)
		}
	})

	result := runReviewFixture(t, fixture, client, Request{
		WorkspaceID: fixture.workspaceID,
		SnapshotID:  fixture.snapshotID,
		Symbol:      "demo.Root",
		MaxDepth:    4,
		MaxSteps:    10,
		OutDir:      fixture.reviewDir,
	})

	if len(result.Flow.Accepted) != 2 {
		t.Fatalf("expected root and child acceptances, got %+v", result.Flow.Accepted)
	}
	if len(result.Flow.Steps) != 2 {
		t.Fatalf("expected cycle to stop after revisiting root, got %+v", result.Flow.Steps)
	}
}

func TestRunHonorsMaxDepthAndWritesLLMErrorArtifact(t *testing.T) {
	fixture := newReviewFixture(t)
	client := reviewClientFunc(func(req llm.ReviewRequest) (llm.ReviewResponse, error) {
		switch req.RootSymbol.CanonicalName {
		case "demo.Root":
			return llm.ReviewResponse{
				Decisions: []llm.HopDecision{
					{
						TargetSymbolID:      "sym-child",
						TargetCanonicalName: "demo.Child",
						Status:              facts.StepAccepted,
						Rationale:           "follow child",
					},
				},
			}, nil
		case "demo.Child":
			return llm.ReviewResponse{
				Decisions: []llm.HopDecision{
					{
						TargetSymbolID:      "sym-grand",
						TargetCanonicalName: "demo.Grand",
						Status:              facts.StepAccepted,
						Rationale:           "depth boundary",
					},
				},
			}, nil
		default:
			return llm.ReviewResponse{}, fmt.Errorf("unexpected symbol %s", req.RootSymbol.CanonicalName)
		}
	})

	result := runReviewFixture(t, fixture, client, Request{
		WorkspaceID: fixture.workspaceID,
		SnapshotID:  fixture.snapshotID,
		Symbol:      "demo.Root",
		MaxDepth:    1,
		MaxSteps:    10,
		OutDir:      fixture.reviewDir,
	})

	if len(result.Flow.Accepted) != 2 {
		t.Fatalf("expected root and child acceptances, got %+v", result.Flow.Accepted)
	}
	if result.Flow.Accepted[1].ToCanonicalName != "demo.Grand" {
		t.Fatalf("expected child to reach grandchild target, got %+v", result.Flow.Accepted[1])
	}
	if _, err := os.Stat(filepath.Join(fixture.reviewDir, "llm_error.json")); !os.IsNotExist(err) {
		t.Fatalf("did not expect error artifact in non-error run: %v", err)
	}

	errorClient := reviewClientFunc(func(req llm.ReviewRequest) (llm.ReviewResponse, error) {
		return llm.ReviewResponse{}, fmt.Errorf("configured llm failed")
	})
	errorResult := runReviewFixture(t, fixture, errorClient, Request{
		WorkspaceID: fixture.workspaceID,
		SnapshotID:  fixture.snapshotID,
		Symbol:      "demo.Root",
		MaxDepth:    0,
		MaxSteps:    10,
		OutDir:      fixture.reviewDir,
	})
	if len(errorResult.Flow.Accepted) != 0 {
		t.Fatalf("expected llm failure to avoid accepted steps, got %+v", errorResult.Flow.Accepted)
	}
	if _, err := os.Stat(filepath.Join(fixture.reviewDir, "llm_error.json")); err != nil {
		t.Fatalf("expected llm error artifact: %v", err)
	}
}

type reviewClientFunc func(llm.ReviewRequest) (llm.ReviewResponse, error)

func (f reviewClientFunc) Review(req llm.ReviewRequest) (llm.ReviewResponse, error) {
	return f(req)
}

type reviewFixture struct {
	rootDir     string
	reviewDir   string
	workspaceID string
	snapshotID  string
}

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()

	rootDir := t.TempDir()
	reviewDir := filepath.Join(rootDir, "workspaces", "ws-1", "snapshots", "snap-1", "review")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(rootDir, "flow.go")), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	filePath := filepath.Join(rootDir, "flow.go")
	content := strings.Join([]string{
		"package demo",
		"",
		"func Root() string {",
		"    return Child()",
		"}",
		"",
		"func Child() string {",
		"    return Grand()",
		"}",
		"",
		"func Grand() string {",
		"    return \"grand\"",
		"}",
		"",
		"func Rejected() string {",
		"    return \"reject\"",
		"}",
		"",
		"func TestRoot() {}",
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
				RepositoryRoot: rootDir,
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
				CanonicalName: "demo.Root",
				Name:          "Root",
				Kind:          "function",
				StartLine:     3,
				EndLine:       5,
			},
			{
				ID:            "sym-child",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.Child",
				Name:          "Child",
				Kind:          "function",
				StartLine:     7,
				EndLine:       9,
			},
			{
				ID:            "sym-grand",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.Grand",
				Name:          "Grand",
				Kind:          "function",
				StartLine:     11,
				EndLine:       13,
			},
			{
				ID:            "sym-rejected",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.Rejected",
				Name:          "Rejected",
				Kind:          "function",
				StartLine:     15,
				EndLine:       17,
			},
			{
				ID:            "sym-test",
				RepositoryID:  "repo-1",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.TestRoot",
				Name:          "TestRoot",
				Kind:          "test_function",
				StartLine:     19,
				EndLine:       19,
			},
		},
		CallCandidates: []facts.CallCandidate{
			{
				ID:                  "call-child",
				SourceSymbolID:      "sym-root",
				TargetSymbolID:      "sym-child",
				TargetCanonicalName: "demo.Child",
				Relationship:        "calls",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.9,
				OrderIndex:          1,
			},
			{
				ID:                  "call-unresolved",
				SourceSymbolID:      "sym-root",
				TargetCanonicalName: "demo.Unresolved",
				Relationship:        "calls",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.5,
				OrderIndex:          2,
			},
			{
				ID:                  "call-rejected",
				SourceSymbolID:      "sym-root",
				TargetSymbolID:      "sym-rejected",
				TargetCanonicalName: "demo.Rejected",
				Relationship:        "test",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.4,
				OrderIndex:          3,
			},
			{
				ID:                  "call-cycle",
				SourceSymbolID:      "sym-child",
				TargetSymbolID:      "sym-root",
				TargetCanonicalName: "demo.Root",
				Relationship:        "calls",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.88,
				OrderIndex:          1,
			},
			{
				ID:                  "call-grand",
				SourceSymbolID:      "sym-child",
				TargetSymbolID:      "sym-grand",
				TargetCanonicalName: "demo.Grand",
				Relationship:        "calls",
				EvidenceType:        "static",
				EvidenceSource:      "tree-sitter-go",
				ExtractionMethod:    "go",
				ConfidenceScore:     0.87,
				OrderIndex:          2,
			},
		},
		Tests: []facts.TestFact{
			{
				ID:            "test-1",
				SymbolID:      "sym-test",
				FileID:        "file-1",
				FilePath:      "flow.go",
				CanonicalName: "demo.TestRoot",
				Name:          "TestRoot",
				StartLine:     19,
				EndLine:       19,
			},
		},
	}

	if err := store.SaveIndex(index); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	return reviewFixture{
		rootDir:     rootDir,
		reviewDir:   reviewDir,
		workspaceID: "ws-1",
		snapshotID:  "snap-1",
	}
}

func runReviewFixture(t *testing.T, fixture reviewFixture, client llm.Client, req Request) Result {
	t.Helper()

	svc := New(fixture.rootDir, factquery.New(fixture.rootDir), client)
	result, err := svc.Run(req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result
}
