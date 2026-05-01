package indexer

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	domainworkspace "analysis-module/internal/domain/workspace"
	"analysis-module/internal/indexer/detector"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	scannerport "analysis-module/internal/ports/scanner"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/pkg/ids"
)

// ScanRequest describes inputs for a workspace scan.
type ScanRequest struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

// ScanResult holds the complete output of a scan including inventory, manifests, and artifacts.
type ScanResult struct {
	WorkspaceManifest domainworkspace.Manifest `json:"workspace_manifest"`
	Inventory         repository.Inventory     `json:"inventory"`
	ServiceManifests  []service.Manifest       `json:"service_manifests"`
	ArtifactRefs      []artifact.Ref           `json:"artifact_refs"`
	Warnings          []string                 `json:"warnings"`
}

// ScanWorkflow orchestrates workspace scanning and inventory building.
type ScanWorkflow struct {
	scanner        WorkspaceScanner
	inventory      InventoryBuilder
	artifactStore  artifactstoreport.Store
	snapshotManage snapshot_manage.Service
}

// WorkspaceScannerService performs repository discovery and file inventory generation.
type WorkspaceScannerService struct {
	repoRootDetector  scannerport.RepoRootDetector
	techStackDetector scannerport.TechStackDetector
	serviceDetector   scannerport.ServiceDetector
	reporter          Reporter
}

func NewWorkspaceScannerService(
	repoRootDetector scannerport.RepoRootDetector,
	techStackDetector scannerport.TechStackDetector,
	serviceDetector scannerport.ServiceDetector,
	reporter Reporter,
) WorkspaceScannerService {
	if reporter == nil {
		reporter = noopReporter{}
	}
	return WorkspaceScannerService{
		repoRootDetector:  repoRootDetector,
		techStackDetector: techStackDetector,
		serviceDetector:   serviceDetector,
		reporter:          reporter,
	}
}

func (s WorkspaceScannerService) Scan(req scannerport.ScanWorkspaceRequest) (scannerport.ScanWorkspaceResult, error) {
	policy := analysis.NewIgnorePolicy(req.IgnorePatterns)
	s.reporter.StartStage("scan", 0)
	repoRoots, err := s.repoRootDetector.Detect(req.WorkspacePath, policy)
	if err != nil {
		s.reporter.FinishStage("scan failed")
		return scannerport.ScanWorkspaceResult{}, err
	}

	repos := make([]repository.Manifest, 0, len(repoRoots))
	warnings := []string{}
	for _, root := range repoRoots {
		s.reporter.Status("repo=" + filepath.Base(root) + " warnings=" + strconv.Itoa(len(warnings)))
		techStack, err := s.techStackDetector.Detect(root, policy)
		if err != nil {
			warnings = append(warnings, "tech stack detection failed for "+root+": "+err.Error())
			continue
		}

		files, err := collectRepoFiles(root, policy)
		if err != nil {
			warnings = append(warnings, "file collection failed for "+root+": "+err.Error())
			continue
		}

		repoManifest := repository.Manifest{
			ID:              repository.ID(ids.Stable("repo", root)),
			Name:            filepath.Base(root),
			RootPath:        root,
			Role:            repository.RoleUnknown,
			IgnoreSignature: policy.Signature,
			TechStack:       techStack,
			GoFiles:         files.goFiles,
			PythonFiles:     files.pythonFiles,
			JavaScriptFiles: files.javascriptFiles,
			TypeScriptFiles: files.typeScriptFiles,
			ConfigFiles:     files.configFiles,
			IssueCounts:     analysis.IssueCounts{SkippedIgnoredFiles: files.skippedCount},
		}

		detectedServices, hints, err := s.serviceDetector.Detect(repoManifest, policy)
		if err != nil {
			warnings = append(warnings, "service detection failed for "+root+": "+err.Error())
		}
		repoManifest.BoundaryHints = hints
		repoManifest.CandidateServices = detectedServices
		repos = append(repos, repoManifest)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].RootPath < repos[j].RootPath
	})
	s.reporter.FinishStage("repos=" + strconv.Itoa(len(repos)) + " warnings=" + strconv.Itoa(len(warnings)))
	return scannerport.ScanWorkspaceResult{
		WorkspacePath: filepath.Clean(req.WorkspacePath),
		Repositories:  repos,
		Warnings:      warnings,
	}, nil
}

func NewScanWorkflow(
	scanner WorkspaceScanner,
	inventory InventoryBuilder,
	artifactStore artifactstoreport.Store,
	snapshotManage snapshot_manage.Service,
) ScanWorkflow {
	return ScanWorkflow{
		scanner:        scanner,
		inventory:      inventory,
		artifactStore:  artifactStore,
		snapshotManage: snapshotManage,
	}
}

// Run executes the scan workflow.
func (w ScanWorkflow) Run(req ScanRequest) (ScanResult, error) {
	workspacePath := filepath.Clean(req.WorkspacePath)
	workspaceID := w.snapshotManage.NewWorkspaceID(workspacePath)
	scanResult, err := w.scanner.Scan(scannerport.ScanWorkspaceRequest{
		WorkspacePath:  workspacePath,
		IgnorePatterns: req.IgnorePatterns,
		TargetHints:    req.TargetHints,
	})
	if err != nil {
		return ScanResult{}, err
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
	manifest := domainworkspace.Manifest{
		ID:              domainworkspace.ID(workspaceID),
		RootPath:        domainworkspace.Path(workspacePath),
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
	return ScanResult{
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

type collectedRepoFiles struct {
	goFiles         []string
	pythonFiles     []string
	javascriptFiles []string
	typeScriptFiles []string
	configFiles     []string
	skippedCount    int
}

func collectRepoFiles(root string, policy analysis.IgnorePolicy) (collectedRepoFiles, error) {
	walkResult, err := detector.Walk(root, policy, nil)
	if err != nil {
		return collectedRepoFiles{}, err
	}
	result := collectedRepoFiles{skippedCount: walkResult.SkippedEntryCount}
	for _, entry := range walkResult.Entries {
		if entry.IsDir {
			continue
		}
		rel, _ := filepath.Rel(root, entry.Path)
		rel = filepath.ToSlash(rel)
		switch strings.ToLower(filepath.Ext(entry.Path)) {
		case ".go":
			result.goFiles = append(result.goFiles, rel)
		case ".py":
			result.pythonFiles = append(result.pythonFiles, rel)
		case ".js", ".jsx", ".mjs", ".cjs":
			result.javascriptFiles = append(result.javascriptFiles, rel)
		case ".ts", ".tsx":
			result.typeScriptFiles = append(result.typeScriptFiles, rel)
		}
		lower := strings.ToLower(rel)
		if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".toml") || strings.HasSuffix(lower, ".proto") {
			result.configFiles = append(result.configFiles, rel)
		}
	}
	sort.Strings(result.goFiles)
	sort.Strings(result.pythonFiles)
	sort.Strings(result.javascriptFiles)
	sort.Strings(result.typeScriptFiles)
	sort.Strings(result.configFiles)
	return result, nil
}
