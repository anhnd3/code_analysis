package workspace_scan

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
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
	s.reporter.StartStage("scan", 0)
	repoRoots, err := s.repoRootDetector.Detect(req.WorkspacePath, req.IgnorePatterns)
	if err != nil {
		s.reporter.FinishStage("scan failed")
		return scannerport.ScanWorkspaceResult{}, err
	}
	repos := make([]repository.Manifest, 0, len(repoRoots))
	warnings := []string{}
	for _, root := range repoRoots {
		s.reporter.Status("repo=" + filepath.Base(root) + " warnings=" + strconv.Itoa(len(warnings)))
		techStack, err := s.techStackDetector.Detect(root)
		if err != nil {
			warnings = append(warnings, "tech stack detection failed for "+root+": "+err.Error())
			continue
		}
		repoManifest := repository.Manifest{
			ID:              repository.ID(ids.Stable("repo", root)),
			Name:            filepath.Base(root),
			RootPath:        root,
			Role:            repository.RoleUnknown,
			TechStack:       techStack,
			GoFiles:         collectGoFiles(root),
			PythonFiles:     collectFilesByExt(root, ".py"),
			JavaScriptFiles: collectFilesByExt(root, ".js", ".jsx", ".mjs", ".cjs"),
			TypeScriptFiles: collectFilesByExt(root, ".ts", ".tsx"),
			ConfigFiles:     collectConfigFiles(root),
		}
		entrypoints, hints, err := s.serviceDetector.Detect(repoManifest)
		if err != nil {
			warnings = append(warnings, "service detection failed for "+root+": "+err.Error())
		}
		repoManifest.BoundaryHints = hints
		if len(entrypoints) > 0 {
			serviceName := repoManifest.Name
			repoManifest.CandidateServices = []service.Manifest{{
				ID:           service.ID(ids.Stable("svc", repoManifest.RootPath, serviceName)),
				Name:         serviceName,
				RepositoryID: string(repoManifest.ID),
				RootPath:     repoManifest.RootPath,
				Entrypoints:  entrypoints,
				Boundaries:   inferBoundaries(hints),
			}}
		}
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

func collectGoFiles(root string) []string {
	return collectFilesByExt(root, ".go")
}

func collectFilesByExt(root string, exts ...string) []string {
	files := []string{}
	extSet := map[string]struct{}{}
	for _, ext := range exts {
		extSet[strings.ToLower(ext)] = struct{}{}
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" || d.Name() == "artifacts" || d.Name() == "__pycache__" || d.Name() == ".venv" || d.Name() == ".pytest_cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := extSet[strings.ToLower(filepath.Ext(path))]; ok {
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func collectConfigFiles(root string) []string {
	files := []string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" || d.Name() == "artifacts" || d.Name() == "__pycache__" || d.Name() == ".venv" || d.Name() == ".pytest_cache" {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(path)
		if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".toml") || strings.HasSuffix(lower, ".proto") {
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func inferBoundaries(hints []repository.BoundaryHint) []service.BoundaryType {
	seen := map[service.BoundaryType]struct{}{}
	for _, hint := range hints {
		switch hint.Type {
		case "grpc":
			seen[service.BoundaryGRPC] = struct{}{}
		case "http":
			seen[service.BoundaryHTTP] = struct{}{}
		case "kafka":
			seen[service.BoundaryKafka] = struct{}{}
		}
	}
	result := make([]service.BoundaryType, 0, len(seen))
	for boundary := range seen {
		result = append(result, boundary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
