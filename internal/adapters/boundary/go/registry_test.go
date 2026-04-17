package boundary

import (
	"testing"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
)

// --- stub detectors for testing ---

type stubDetector struct {
	name  string
	roots []boundaryroot.Root
}

func (s *stubDetector) Name() string { return s.name }
func (s *stubDetector) DetectBoundaries(_ ParsedGoFile, _ []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	return s.roots, nil
}

func makeRoot(sourceFile, method, path, handler, confidence, framework string) boundaryroot.Root {
	return boundaryroot.Root{
		ID:            framework + ":" + method + ":" + path,
		Kind:          boundaryroot.KindHTTP,
		Framework:     framework,
		Method:        method,
		Path:          path,
		CanonicalName: method + " " + path,
		HandlerTarget: handler,
		SourceFile:    sourceFile,
		Confidence:    confidence,
	}
}

// TestDetectAll_NoDuplicates verifies that distinct roots pass through unchanged.
func TestDetectAll_NoDuplicates(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubDetector{
		name: "gin",
		roots: []boundaryroot.Root{
			makeRoot("main.go", "GET", "/users", "ListUsers", "high", "gin"),
			makeRoot("main.go", "POST", "/users", "CreateUser", "high", "gin"),
		},
	})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

// TestDetectAll_DeduplicatesIdenticalKey verifies that when two detectors report the same
// (SourceFile, Method, Path, HandlerTarget), only one root is returned.
func TestDetectAll_DeduplicatesIdenticalKey(t *testing.T) {
	root := makeRoot("main.go", "GET", "/users", "ListUsers", "high", "gin")
	dupRoot := makeRoot("main.go", "GET", "/users", "ListUsers", "medium", "net/http")

	r := NewRegistry()
	r.Register(&stubDetector{name: "gin", roots: []boundaryroot.Root{root}})
	r.Register(&stubDetector{name: "net/http", roots: []boundaryroot.Root{dupRoot}})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 deduplicated result, got %d", len(results))
	}
}

// TestDetectAll_HigherConfidenceWins verifies that when two detectors collide,
// the one with higher confidence is kept.
func TestDetectAll_HigherConfidenceWins(t *testing.T) {
	lowRoot := makeRoot("main.go", "GET", "/orders", "GetOrders", "low", "net/http")
	highRoot := makeRoot("main.go", "GET", "/orders", "GetOrders", "high", "gin")

	r := NewRegistry()
	// Register low-confidence first.
	r.Register(&stubDetector{name: "net/http", roots: []boundaryroot.Root{lowRoot}})
	r.Register(&stubDetector{name: "gin", roots: []boundaryroot.Root{highRoot}})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Root.Confidence != "high" {
		t.Errorf("expected winner confidence 'high', got %q", results[0].Root.Confidence)
	}
	if results[0].Detector != "gin" {
		t.Errorf("expected winner detector 'gin', got %q", results[0].Detector)
	}
}

// TestDetectAll_EqualConfidenceEmitsDiagnostic verifies that when two detectors
// collide with equal confidence, an ambiguity diagnostic is emitted and the
// first-seen root is kept.
func TestDetectAll_EqualConfidenceEmitsDiagnostic(t *testing.T) {
	root1 := makeRoot("main.go", "GET", "/health", "HealthA", "medium", "gin")
	root2 := makeRoot("main.go", "GET", "/health", "HealthA", "medium", "net/http")

	r := NewRegistry()
	r.Register(&stubDetector{name: "gin", roots: []boundaryroot.Root{root1}})
	r.Register(&stubDetector{name: "net/http", roots: []boundaryroot.Root{root2}})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// First-seen (gin) wins.
	if results[0].Detector != "gin" {
		t.Errorf("expected first-seen detector 'gin', got %q", results[0].Detector)
	}
	// Must have an ambiguity diagnostic.
	hasAmbiguity := false
	for _, d := range results[0].Diagnostics {
		if d.Category == "boundary_ambiguity" {
			hasAmbiguity = true
			break
		}
	}
	if !hasAmbiguity {
		t.Error("expected boundary_ambiguity diagnostic for equal-confidence collision, got none")
	}
}

// TestDetectAll_MultipleCollisions verifies ordering is stable and only unique keys survive.
func TestDetectAll_MultipleCollisions(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubDetector{
		name: "gin",
		roots: []boundaryroot.Root{
			makeRoot("main.go", "GET", "/a", "HandlerA", "high", "gin"),
			makeRoot("main.go", "POST", "/b", "HandlerB", "high", "gin"),
		},
	})
	r.Register(&stubDetector{
		name: "net/http",
		roots: []boundaryroot.Root{
			// Overlaps with gin's /a
			makeRoot("main.go", "GET", "/a", "HandlerA", "medium", "net/http"),
			// Unique
			makeRoot("main.go", "DELETE", "/c", "HandlerC", "medium", "net/http"),
		},
	})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 3 {
		t.Fatalf("expected 3 unique results, got %d", len(results))
	}
}

// TestConfidenceRank ensures the ranking function is monotone.
func TestConfidenceRank(t *testing.T) {
	if confidenceRank("high") <= confidenceRank("medium") {
		t.Error("expected high > medium")
	}
	if confidenceRank("medium") <= confidenceRank("low") {
		t.Error("expected medium > low")
	}
	if confidenceRank("unknown") != confidenceRank("low") {
		t.Error("expected unknown to have the same rank as low")
	}
}
