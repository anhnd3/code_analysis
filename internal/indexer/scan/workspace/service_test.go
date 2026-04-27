package workspace_scan

import (
	"testing"

	scannerdetectors "analysis-module/internal/adapters/scanner/detectors"
	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/repository"
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

func TestScanRespectsIgnorePolicyAcrossRepoInventory(t *testing.T) {
	service := New(
		scannerdetectors.NewRepoRootDetector(progress.NoopReporter{}),
		scannerdetectors.NewTechStackDetector(),
		scannerdetectors.NewServiceDetector(),
		progress.NoopReporter{},
	)
	result, err := service.Scan(scannerport.ScanWorkspaceRequest{
		WorkspacePath:  fixtures.WorkspacePath(t, "ignore_policy_app"),
		IgnorePatterns: []string{"ignored"},
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(result.Repositories))
	}
	repo := result.Repositories[0]
	if len(repo.JavaScriptFiles) != 0 || len(repo.TypeScriptFiles) != 0 {
		t.Fatalf("expected ignored js/ts files to be excluded, got js=%d ts=%d", len(repo.JavaScriptFiles), len(repo.TypeScriptFiles))
	}
	if repo.IssueCounts.SkippedIgnoredFiles == 0 {
		t.Fatal("expected skipped ignored file count to be recorded")
	}
}

func TestServiceDetectorFindsPythonAndNodeServices(t *testing.T) {
	detector := scannerdetectors.NewServiceDetector()
	policy := analysis.NewIgnorePolicy(nil)

	pythonRepo := repoFromFixture(t, "python_service_app")
	pythonServices, _, err := detector.Detect(pythonRepo, policy)
	if err != nil {
		t.Fatalf("detect python service: %v", err)
	}
	if len(pythonServices) == 0 {
		t.Fatal("expected at least one python service candidate")
	}

	nodeRepo := repoFromFixture(t, "node_service_app")
	nodeServices, _, err := detector.Detect(nodeRepo, policy)
	if err != nil {
		t.Fatalf("detect node service: %v", err)
	}
	if len(nodeServices) == 0 {
		t.Fatal("expected at least one node service candidate")
	}
}

func repoFromFixture(t *testing.T, name string) repository.Manifest {
	t.Helper()
	root := fixtures.WorkspacePath(t, name)
	return repository.Manifest{
		ID:       repository.ID("repo_" + name),
		Name:     name,
		RootPath: root,
	}
}
