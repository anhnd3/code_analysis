package indexer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"analysis-module/internal/tests/fixtures"
)

// testArtifactStore is a minimal in-memory artifact store for tests.
type testArtifactStore struct {
	root string
}

func (s testArtifactStore) SaveJSON(workspaceID, snapshotID, fileName string, artifactType ArtifactType, payload any) (ArtifactRef, error) {
	path := filepath.Join(s.root, "workspaces", workspaceID, "snapshots", snapshotID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ArtifactRef{}, err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ArtifactRef{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ArtifactRef{}, err
	}
	return ArtifactRef{Type: artifactType, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: path}, nil
}

func (s testArtifactStore) SaveText(workspaceID, snapshotID, fileName string, artifactType ArtifactType, body string) (ArtifactRef, error) {
	path := filepath.Join(s.root, "workspaces", workspaceID, "snapshots", snapshotID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ArtifactRef{}, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return ArtifactRef{}, err
	}
	return ArtifactRef{Type: artifactType, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: path}, nil
}

func newTestArtifactStore(root string) testArtifactStore {
	return testArtifactStore{root: root}
}

func TestWorkspaceScannerServiceFindsMultipleRepositories(t *testing.T) {
	service := NewWorkspaceScannerService(
		NewRepoRootDetector(noopReporter{}),
		NewTechStackDetector(),
		NewServiceDetector(),
		noopReporter{},
	)
	result, err := service.Scan(ScanWorkspaceRequest{
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
		NewRepoRootDetector(noopReporter{}),
		NewTechStackDetector(),
		NewServiceDetector(),
		noopReporter{},
	)
	result, err := service.Scan(ScanWorkspaceRequest{
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
		NewRepoRootDetector(noopReporter{}),
		NewTechStackDetector(),
		NewServiceDetector(),
		noopReporter{},
	)
	result, err := service.Scan(ScanWorkspaceRequest{
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
	serviceDetector := NewServiceDetector()
	policy := NewIgnorePolicy(nil)

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
		ArtifactStore:  newTestArtifactStore(artifactRoot),
		SnapshotManage: NewSnapshotService(),
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

func repoFromFixture(t *testing.T, name string) Manifest {
	t.Helper()
	root := fixtures.WorkspacePath(t, name)
	return Manifest{
		ID:       RepoID("repo_" + name),
		Name:     name,
		RootPath: root,
	}
}
