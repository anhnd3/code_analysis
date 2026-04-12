package goextractor

import (
	"path/filepath"
	"testing"

	"analysis-module/internal/domain/symbol"
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
