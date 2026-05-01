package boundary

import (
	"testing"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
)

type stubDetector struct {
	name  string
	roots []boundaryroot.Root
}

func (s *stubDetector) Name() string { return s.name }
func (s *stubDetector) DetectBoundaries(_ ParsedGoFile, _ []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic) {
	return s.roots, nil
}

func makeRoot(repoID, sourceFile, method, path, handler, confidence, framework string, start, end uint32) boundaryroot.Root {
	root := boundaryroot.Root{
		Kind:            boundaryroot.KindHTTP,
		Framework:       framework,
		Method:          method,
		Path:            path,
		CanonicalName:   method + " " + path,
		HandlerTarget:   handler,
		RepositoryID:    repoID,
		SourceFile:      sourceFile,
		SourceStartByte: start,
		SourceEndByte:   end,
		Confidence:      confidence,
	}
	root.ID = boundaryroot.StableID(root)
	return root
}

func TestDetectAll_DeduplicatesIdenticalRootKey(t *testing.T) {
	root := makeRoot("repo1", "main.go", "GET", "/users", "ListUsers", "high", "gin", 10, 30)
	dup := makeRoot("repo1", "main.go", "GET", "/users", "ListUsers", "medium", "net/http", 10, 30)

	r := NewRegistry()
	r.Register(&stubDetector{name: "gin", roots: []boundaryroot.Root{root}})
	r.Register(&stubDetector{name: "net/http", roots: []boundaryroot.Root{dup}})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 deduplicated result, got %d", len(results))
	}
	if results[0].Detector != "gin" {
		t.Fatalf("expected higher-confidence detector to win, got %q", results[0].Detector)
	}
}

func TestDetectAll_DistinctSpansDoNotCollide(t *testing.T) {
	first := makeRoot("repo1", "main.go", "POST", "/detect-qr", "DetectQRV1", "high", "gin", 40, 70)
	second := makeRoot("repo1", "main.go", "POST", "/detect-qr", "DetectQRV2", "high", "gin", 90, 120)

	r := NewRegistry()
	r.Register(&stubDetector{name: "gin", roots: []boundaryroot.Root{first, second}})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 2 {
		t.Fatalf("expected same method/path at different spans to remain distinct, got %d", len(results))
	}
	if results[0].Root.ID == results[1].Root.ID {
		t.Fatalf("expected distinct stable IDs for distinct spans, got %q", results[0].Root.ID)
	}
}

func TestDetectAll_DeterministicWinnerSelection(t *testing.T) {
	ginRoot := makeRoot("repo1", "main.go", "GET", "/health", "HealthHandler", "high", "gin", 10, 20)
	netHTTPRoot := makeRoot("repo1", "main.go", "GET", "/health", "HealthHandler", "high", "net/http", 10, 20)

	r := NewRegistry()
	r.Register(&stubDetector{name: "net/http", roots: []boundaryroot.Root{netHTTPRoot}})
	r.Register(&stubDetector{name: "gin", roots: []boundaryroot.Root{ginRoot}})

	results := r.DetectAll(ParsedGoFile{}, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Detector != "gin" {
		t.Fatalf("expected deterministic detector winner, got %+v", results[0])
	}
}

func TestDetectAll_OutputOrderIndependentOfDetectorRegistration(t *testing.T) {
	rootsA := []boundaryroot.Root{
		makeRoot("repo1", "routes.go", "GET", "/health", "Health", "high", "gin", 10, 20),
		makeRoot("repo1", "routes.go", "POST", "/v1/camera/detect-qr", "DetectQR", "high", "gin", 30, 60),
	}
	rootsB := []boundaryroot.Root{
		makeRoot("repo1", "routes.go", "GET", "/health", "Health", "medium", "net/http", 10, 20),
		makeRoot("repo1", "routes.go", "GET", "/metrics/prom", "gin.WrapH", "high", "gin", 80, 120),
	}

	first := NewRegistry()
	first.Register(&stubDetector{name: "gin", roots: rootsA})
	first.Register(&stubDetector{name: "net/http", roots: rootsB})

	second := NewRegistry()
	second.Register(&stubDetector{name: "net/http", roots: rootsB})
	second.Register(&stubDetector{name: "gin", roots: rootsA})

	firstResults := first.DetectAll(ParsedGoFile{}, nil)
	secondResults := second.DetectAll(ParsedGoFile{}, nil)

	if len(firstResults) != len(secondResults) {
		t.Fatalf("expected equal result counts, got %d vs %d", len(firstResults), len(secondResults))
	}
	for i := range firstResults {
		if firstResults[i].Root.ID != secondResults[i].Root.ID {
			t.Fatalf("expected stable output ordering independent of detector registration order:\nfirst=%+v\nsecond=%+v", firstResults, secondResults)
		}
	}
}

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
