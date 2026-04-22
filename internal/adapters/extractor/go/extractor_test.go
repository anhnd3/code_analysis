package goextractor

import (
	"os"
	"path/filepath"
	"testing"

	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/domain/targetref"
	"analysis-module/internal/tests/fixtures"
)

func TestExtractorParsesGoFixture(t *testing.T) {
	extractor := New()
	workspace := fixtures.WorkspacePath(t, "single_go_service")
	file := symbol.FileRef{
		RepositoryID: "single_go_service",
		AbsolutePath: filepath.Join(workspace, "internal", "service", "service.go"),
		RelativePath: "internal/service/service.go",
		Language:     "go",
	}

	result, err := extractor.ExtractFile(file)
	if err != nil {
		t.Fatalf("extract file: %v", err)
	}
	if result.PackageName != "service" {
		t.Fatalf("expected package service, got %q", result.PackageName)
	}
	if len(result.Imports) != 1 || result.Imports[0] != "example.com/single/internal/helper" {
		t.Fatalf("unexpected imports: %#v", result.Imports)
	}

	var sawHandle bool
	var sawStepOne bool
	var sawHelperCall bool
	for _, sym := range result.Symbols {
		if sym.Name == "Handle" && sym.Kind == symbol.KindFunction {
			sawHandle = true
		}
		if sym.Name == "stepOne" && sym.Kind == symbol.KindFunction {
			sawStepOne = true
		}
	}
	for _, rel := range result.Relations {
		if rel.TargetCanonicalName == "example.com/single/internal/helper.Transform" {
			sawHelperCall = true
		}
	}

	if !sawHandle || !sawStepOne || !sawHelperCall {
		t.Fatalf("missing extracted data: handle=%v stepOne=%v helperCall=%v", sawHandle, sawStepOne, sawHelperCall)
	}
}

func TestExtractorFindsTestFunctions(t *testing.T) {
	extractor := New()
	workspace := fixtures.WorkspacePath(t, "single_go_service")
	file := symbol.FileRef{
		RepositoryID: "single_go_service",
		AbsolutePath: filepath.Join(workspace, "internal", "service", "service_test.go"),
		RelativePath: "internal/service/service_test.go",
		Language:     "go",
	}

	result, err := extractor.ExtractFile(file)
	if err != nil {
		t.Fatalf("extract file: %v", err)
	}

	var sawTest bool
	var sawHandleCall bool
	for _, sym := range result.Symbols {
		if sym.Name == "TestHandle" && sym.Kind == symbol.KindTestFunction {
			sawTest = true
		}
	}
	for _, rel := range result.Relations {
		if rel.TargetCanonicalName == "service.Handle" {
			sawHandleCall = true
		}
	}

	if !sawTest || !sawHandleCall {
		t.Fatalf("missing extracted test data: test=%v handleCall=%v", sawTest, sawHandleCall)
	}
}

func TestExtractorEmitsReceiverAwareSelectorTargets(t *testing.T) {
	extractor := New()
	workspace := t.TempDir()

	writeTestFile(t, filepath.Join(workspace, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"))
	writeTestFile(t, filepath.Join(workspace, "repo", "repo.go"), []byte("package repo\n\ntype CameraRepo struct{}\n"))
	writeTestFile(t, filepath.Join(workspace, "session", "session.go"), []byte("package session\n\ntype Service interface{}\n"))
	writeTestFile(t, filepath.Join(workspace, "handler.go"), []byte(`package handler

import (
	"fmt"

	"example.com/demo/repo"
	"example.com/demo/session"
)

type handler struct {
	repo    *repo.CameraRepo
	session session.Service
}

func (h *handler) helper() {}

func (h *handler) Handle() {
	h.helper()
	h.repo.DetectQR()
	h.session.GetSessionByZlpToken()
	fmt.Println("ok")
}
`))

	result, err := extractor.ExtractFile(symbol.FileRef{
		RepositoryID:   "demo",
		RepositoryRoot: workspace,
		AbsolutePath:   filepath.Join(workspace, "handler.go"),
		RelativePath:   "handler.go",
		Language:       "go",
	})
	if err != nil {
		t.Fatalf("extract file: %v", err)
	}

	assertRelationTarget(t, result.Relations, "h.helper", "handler.handler.helper", targetref.KindExactCanonical, "receiver_selector")
	assertRelationTarget(t, result.Relations, "h.repo.DetectQR", "repo.DetectQR", targetref.KindPackageMethodHint, "receiver_field_selector")
	assertRelationTarget(t, result.Relations, "h.session.GetSessionByZlpToken", "session.GetSessionByZlpToken", targetref.KindPackageMethodHint, "receiver_field_selector")
	assertRelationTarget(t, result.Relations, "fmt.Println", "fmt.Println", targetref.KindExactCanonical, "import_selector")
	assertRelationOrder(t, result.Relations, "h.helper", 0)
	assertRelationOrder(t, result.Relations, "h.repo.DetectQR", 1)
	assertRelationOrder(t, result.Relations, "h.session.GetSessionByZlpToken", 2)
	assertRelationOrder(t, result.Relations, "fmt.Println", 3)
}

func TestExtractorUsesSiblingPackageTypeInfoForReceiverFieldSelectors(t *testing.T) {
	extractor := New()
	workspace := t.TempDir()

	writeTestFile(t, filepath.Join(workspace, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"))
	writeTestFile(t, filepath.Join(workspace, "repo", "repo.go"), []byte("package repo\n\ntype CameraRepo interface{}\n"))
	writeTestFile(t, filepath.Join(workspace, "handler", "types.go"), []byte(`package handler

import "example.com/demo/repo"

type handler struct {
	repo repo.CameraRepo
}
`))
	writeTestFile(t, filepath.Join(workspace, "handler", "logic.go"), []byte(`package handler

func (h *handler) Handle() {
	h.repo.DetectQR()
}
`))

	result, err := extractor.ExtractFile(symbol.FileRef{
		RepositoryID:   "demo",
		RepositoryRoot: workspace,
		AbsolutePath:   filepath.Join(workspace, "handler", "logic.go"),
		RelativePath:   "handler/logic.go",
		Language:       "go",
	})
	if err != nil {
		t.Fatalf("extract file: %v", err)
	}

	assertRelationTarget(t, result.Relations, "h.repo.DetectQR", "repo.DetectQR", targetref.KindPackageMethodHint, "receiver_field_selector")
}

func assertRelationTarget(t *testing.T, relations []symbol.RelationCandidate, evidenceSource, target string, kind targetref.Kind, evidenceType string) {
	t.Helper()
	for _, relation := range relations {
		if relation.EvidenceSource != evidenceSource {
			continue
		}
		if relation.TargetCanonicalName != target {
			t.Fatalf("expected target %q for %s, got %+v", target, evidenceSource, relation)
		}
		if relation.TargetKind != kind {
			t.Fatalf("expected target kind %s for %s, got %+v", kind, evidenceSource, relation)
		}
		if relation.EvidenceType != evidenceType {
			t.Fatalf("expected evidence type %q for %s, got %+v", evidenceType, evidenceSource, relation)
		}
		return
	}
	t.Fatalf("missing relation for %s in %+v", evidenceSource, relations)
}

func assertRelationOrder(t *testing.T, relations []symbol.RelationCandidate, evidenceSource string, orderIndex int) {
	t.Helper()
	for _, relation := range relations {
		if relation.EvidenceSource != evidenceSource {
			continue
		}
		if relation.OrderIndex != orderIndex {
			t.Fatalf("expected order index %d for %s, got %+v", orderIndex, evidenceSource, relation)
		}
		return
	}
	t.Fatalf("missing relation for %s in %+v", evidenceSource, relations)
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
