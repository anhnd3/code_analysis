package scanner

import (
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
)

type ScanWorkspaceRequest struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

type ScanWorkspaceResult struct {
	WorkspacePath string                `json:"workspace_path"`
	Repositories  []repository.Manifest `json:"repositories"`
	Warnings      []string              `json:"warnings"`
}

type WorkspaceScanner interface {
	Scan(req ScanWorkspaceRequest) (ScanWorkspaceResult, error)
}

type RepoRootDetector interface {
	Detect(root string, policy analysis.IgnorePolicy) ([]string, error)
}

type TechStackDetector interface {
	Detect(repoPath string, policy analysis.IgnorePolicy) (repository.TechStackProfile, error)
}

type ServiceDetector interface {
	Detect(repo repository.Manifest, policy analysis.IgnorePolicy) ([]service.Manifest, []repository.BoundaryHint, error)
}
