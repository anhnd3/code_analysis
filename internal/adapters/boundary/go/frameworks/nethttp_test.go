package frameworks

import (
	"slices"
	"testing"

	"analysis-module/internal/domain/boundaryroot"
)

func TestNetHTTPDetectorUsesBoundedServeMuxProvenance(t *testing.T) {
	files := parseGinFiles(t, map[string][]byte{
		"main.go": []byte(`package main

import "net/http"

type fakeMux struct{}

type ServeMux struct{}

func (*ServeMux) HandleFunc(string, func(http.ResponseWriter, *http.Request)) {}

func (fakeMux) HandleFunc(string, func(http.ResponseWriter, *http.Request)) {}

type server struct {
	mux *http.ServeMux
}

type localServer struct {
	mux *ServeMux
}

func listUsers(http.ResponseWriter, *http.Request) {}

func makeUsersHandler() http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

func (s *server) register() {
	http.HandleFunc("/users", listUsers)

	mux := http.NewServeMux()
	mux.HandleFunc("/factory", makeUsersHandler())

	alias := mux
	alias.HandleFunc("/alias", listUsers)

	s.mux.HandleFunc("/field", listUsers)

	local := localServer{}
	local.mux.HandleFunc("/shadow", listUsers)

	fake := fakeMux{}
	fake.HandleFunc("/bad", listUsers)
}`),
	})

	detector := NewNetHTTPDetector()
	if diags := detector.PreparePackage(files, nil); len(diags) != 0 {
		t.Fatalf("expected no preparation diagnostics, got %+v", diags)
	}

	roots, diags := detector.DetectBoundaries(files[0], nil)
	if len(roots) != 4 {
		t.Fatalf("expected 4 net/http roots, got %d: %+v", len(roots), roots)
	}

	if got := rootByCanonicalName(t, roots, "ANY /users").HandlerTarget; got != "listUsers" {
		t.Fatalf("expected direct handler target, got %q", got)
	}
	if got := rootByCanonicalName(t, roots, "ANY /factory").HandlerTarget; got != "makeUsersHandler" {
		t.Fatalf("expected factory handler target to preserve outer callee, got %q", got)
	}
	if got := rootByCanonicalName(t, roots, "ANY /alias").HandlerTarget; got != "listUsers" {
		t.Fatalf("expected same-block alias route target, got %q", got)
	}
	fieldRoot := rootByCanonicalName(t, roots, "ANY /field")
	if fieldRoot.HandlerTarget != "listUsers" {
		t.Fatalf("expected field-backed ServeMux target, got %q", fieldRoot.HandlerTarget)
	}
	if fieldRoot.ID == "" || fieldRoot.ID != boundaryroot.StableID(fieldRoot) {
		t.Fatalf("expected stable root ID for %+v", fieldRoot)
	}

	diagCategories := collectDiagnosticCategories(diags)
	if !slices.Contains(diagCategories, "boundary_unproven_receiver") {
		t.Fatalf("expected unproven receiver diagnostic for fake mux, got %+v", diags)
	}
}
