package workspace_scan

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	adapterfs "analysis-module/internal/adapters/scanner/filesystem"
	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/repository"
	scannerport "analysis-module/internal/ports/scanner"
	"analysis-module/pkg/ids"
)

type Service struct {
	repoRootDetector  scannerport.RepoRootDetector
	techStackDetector scannerport.TechStackDetector
	serviceDetector   scannerport.ServiceDetector
	reporter          progress.Reporter
}

func New(repoRootDetector scannerport.RepoRootDetector, techStackDetector scannerport.TechStackDetector, serviceDetector scannerport.ServiceDetector, reporter progress.Reporter) Service {
	if reporter == nil {
		reporter = progress.NoopReporter{}
	}
	return Service{
		repoRootDetector:  repoRootDetector,
		techStackDetector: techStackDetector,
		serviceDetector:   serviceDetector,
		reporter:          reporter,
	}
}

func (s Service) Scan(req scannerport.ScanWorkspaceRequest) (scannerport.ScanWorkspaceResult, error) {
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
		services, hints, err := s.serviceDetector.Detect(repoManifest, policy)
		if err != nil {
			warnings = append(warnings, "service detection failed for "+root+": "+err.Error())
		}
		repoManifest.BoundaryHints = hints
		repoManifest.CandidateServices = services
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

type collectedRepoFiles struct {
	goFiles         []string
	pythonFiles     []string
	javascriptFiles []string
	typeScriptFiles []string
	configFiles     []string
	skippedCount    int
}

func collectRepoFiles(root string, policy analysis.IgnorePolicy) (collectedRepoFiles, error) {
	walkResult, err := adapterfs.Walk(root, policy, nil)
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
