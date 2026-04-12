package workspace_scan

import (
	"testing"

	scannerdetectors "analysis-module/internal/adapters/scanner/detectors"
	"analysis-module/internal/app/progress"
	scannerport "analysis-module/internal/ports/scanner"
	"analysis-module/internal/tests/fixtures"
)

func TestScanFindsMultipleRepositories(t *testing.T) {
	service := New(
		scannerdetectors.NewRepoRootDetector(progress.NoopReporter{}),
		scannerdetectors.NewTechStackDetector(),
		scannerdetectors.NewServiceDetector(),
		progress.NoopReporter{},
	)
	result, err := service.Scan(scannerport.ScanWorkspaceRequest{
		WorkspacePath: fixtures.WorkspacePath(t, "multi_repo_discovery"),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Repositories) < 2 {
		t.Fatalf("expected at least 2 repositories, got %d", len(result.Repositories))
	}
}

func TestScanDetectsPythonAndNodeSourceFiles(t *testing.T) {
	service := New(
		scannerdetectors.NewRepoRootDetector(progress.NoopReporter{}),
		scannerdetectors.NewTechStackDetector(),
		scannerdetectors.NewServiceDetector(),
		progress.NoopReporter{},
	)
	result, err := service.Scan(scannerport.ScanWorkspaceRequest{
		WorkspacePath: fixtures.WorkspacePath(t, "mixed_language_app"),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(result.Repositories))
	}
	repo := result.Repositories[0]
	if len(repo.PythonFiles) == 0 {
		t.Fatal("expected python files to be discovered")
	}
	if len(repo.JavaScriptFiles) == 0 {
		t.Fatal("expected javascript files to be discovered")
	}
	if len(repo.TypeScriptFiles) == 0 {
		t.Fatal("expected typescript files to be discovered")
	}
}
