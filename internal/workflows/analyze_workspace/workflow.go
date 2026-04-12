package analyze_workspace

import (
	"path/filepath"
	"sort"
	"time"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/workspace"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	scannerport "analysis-module/internal/ports/scanner"
	"analysis-module/internal/services/repo_inventory"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/services/workspace_scan"
)

type Request struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

type Result struct {
	WorkspaceManifest workspace.Manifest   `json:"workspace_manifest"`
	Inventory         repository.Inventory `json:"inventory"`
	ServiceManifests  []service.Manifest   `json:"service_manifests"`
	ArtifactRefs      []artifact.Ref       `json:"artifact_refs"`
	Warnings          []string             `json:"warnings"`
}

type Workflow struct {
	scanner        workspace_scan.Service
	inventory      repo_inventory.Service
	artifactStore  artifactstoreport.Store
	snapshotManage snapshot_manage.Service
}

func New(scanner workspace_scan.Service, inventory repo_inventory.Service, artifactStore artifactstoreport.Store, snapshotManage snapshot_manage.Service) Workflow {
	return Workflow{
		scanner:        scanner,
		inventory:      inventory,
		artifactStore:  artifactStore,
		snapshotManage: snapshotManage,
	}
}

func (w Workflow) Run(req Request) (Result, error) {
	workspacePath := filepath.Clean(req.WorkspacePath)
	workspaceID := w.snapshotManage.NewWorkspaceID(workspacePath)
	scanResult, err := w.scanner.Scan(scannerport.ScanWorkspaceRequest{
		WorkspacePath:  workspacePath,
		IgnorePatterns: req.IgnorePatterns,
		TargetHints:    req.TargetHints,
	})
	if err != nil {
		return Result{}, err
	}
	inventory := w.inventory.Build(workspaceID, scanResult)
	repoIDs := make([]string, 0, len(inventory.Repositories))
	languageSet := map[string]struct{}{}
	for _, repo := range inventory.Repositories {
		repoIDs = append(repoIDs, string(repo.ID))
		for _, lang := range repo.TechStack.Languages {
			languageSet[string(lang)] = struct{}{}
		}
	}
	languages := make([]string, 0, len(languageSet))
	for lang := range languageSet {
		languages = append(languages, lang)
	}
	sort.Strings(repoIDs)
	sort.Strings(languages)
	warnings := append([]string{}, scanResult.Warnings...)
	manifest := workspace.Manifest{
		ID:              workspace.ID(workspaceID),
		RootPath:        workspace.Path(workspacePath),
		IgnoreSignature: inventory.IgnoreSignature,
		RepositoryIDs:   repoIDs,
		Languages:       languages,
		Warnings:        warnings,
		ScannedAt:       time.Now().UTC(),
	}
	serviceManifests := collectServiceManifests(inventory.Repositories)
	artifactRefs := []artifact.Ref{}
	if ref, err := w.artifactStore.SaveJSON(workspaceID, "", "workspace_manifest.json", artifact.TypeWorkspaceManifest, manifest); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(workspaceID, "", "repository_manifests.json", artifact.TypeRepositoryManifests, inventory.Repositories); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(workspaceID, "", "service_manifests.json", artifact.TypeServiceManifests, serviceManifests); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(workspaceID, "", "scan_warnings.json", artifact.TypeScanWarnings, warnings); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	return Result{
		WorkspaceManifest: manifest,
		Inventory:         inventory,
		ServiceManifests:  serviceManifests,
		ArtifactRefs:      artifactRefs,
		Warnings:          warnings,
	}, nil
}

func collectServiceManifests(repositories []repository.Manifest) []service.Manifest {
	seen := map[string]service.Manifest{}
	for _, repo := range repositories {
		for _, candidate := range repo.CandidateServices {
			seen[string(candidate.ID)] = candidate
		}
	}
	result := make([]service.Manifest, 0, len(seen))
	for _, manifest := range seen {
		result = append(result, manifest)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RepositoryID == result[j].RepositoryID {
			if result[i].RootPath == result[j].RootPath {
				return result[i].Name < result[j].Name
			}
			return result[i].RootPath < result[j].RootPath
		}
		return result[i].RepositoryID < result[j].RepositoryID
	})
	return result
}
