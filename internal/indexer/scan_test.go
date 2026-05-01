package indexer

import (
	"testing"

	artifactfs "analysis-module/internal/adapters/artifactstore/filesystem"
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/indexer/detector"
	scannerport "analysis-module/internal/ports/scanner"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/tests/fixtures"
)

func TestWorkspaceScannerServiceFindsMultipleRepositories(t *testing.T) {
	service := NewWorkspaceScannerService(
		detector.NewRepoRootDetector(noopReporter{}),
		detector.NewTechStackDetector(),
		detector.NewServiceDetector(),
		noopReporter{},
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

func TestWorkspaceScannerServiceDetectsPythonAndNodeSourceFiles(t *testing.T) {
	service := NewWorkspaceScannerService(
		detector.NewRepoRootDetector(noopReporter{}),
		detector.NewTechStackDetector(),
		detector.NewServiceDetector(),
		noopReporter{},
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

func TestWorkspaceScannerServiceRespectsIgnorePolicyAcrossRepoInventory(t *testing.T) {
	service := NewWorkspaceScannerService(
		detector.NewRepoRootDetector(noopReporter{}),
		detector.NewTechStackDetector(),
		detector.NewServiceDetector(),
		noopReporter{},
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
	serviceDetector := detector.NewServiceDetector()
	policy := analysis.NewIgnorePolicy(nil)

	pythonRepo := repoFromFixture(t, "python_service_app")
	pythonServices, _, err := serviceDetector.Detect(pythonRepo, policy)
	if err != nil {
		t.Fatalf("detect python service: %v", err)
	}
	if len(pythonServices) == 0 {
		t.Fatal("expected at least one python service candidate")
	}

	nodeRepo := repoFromFixture(t, "node_service_app")
	nodeServices, _, err := serviceDetector.Detect(nodeRepo, policy)
	if err != nil {
		t.Fatalf("detect node service: %v", err)
	}
	if len(nodeServices) == 0 {
		t.Fatal("expected at least one node service candidate")
	}
}

func TestDefaultWorkflowsScanAndIndexSingleGoService(t *testing.T) {
	artifactRoot := t.TempDir()
	workflows, err := NewDefaultWorkflows(WorkflowOptions{
		ArtifactRoot:   artifactRoot,
		ArtifactStore:  artifactfs.New(artifactRoot),
		SnapshotManage: snapshot_manage.New(),
		Reporter:       noopReporter{},
	})
	if err != nil {
		t.Fatalf("new default workflows: %v", err)
	}

	workspacePath := fixtures.WorkspacePath(t, "single_go_service")
	scanResult, err := workflows.Scan.Run(ScanRequest{WorkspacePath: workspacePath})
	if err != nil {
		t.Fatalf("scan run: %v", err)
	}
	if len(scanResult.Inventory.Repositories) == 0 {
		t.Fatal("expected at least one scanned repository")
	}
	if len(scanResult.Inventory.Repositories[0].GoFiles) == 0 {
		t.Fatal("expected Go files to be captured in inventory")
	}
	if len(scanResult.Inventory.Plans) == 0 {
		t.Fatal("expected extraction plans for Go files")
	}

	indexResult, err := workflows.Index.Run(IndexRequest{WorkspacePath: workspacePath})
	if err != nil {
		t.Fatalf("index run: %v", err)
	}
	if indexResult.FileCount == 0 {
		t.Fatal("expected indexed files > 0")
	}
	if indexResult.SymbolCount == 0 {
		t.Fatal("expected indexed symbols > 0")
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
