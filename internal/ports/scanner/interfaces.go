package scanner

import "analysis-module/internal/domain/repository"

type ScanWorkspaceRequest struct {
	WorkspacePath string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints   []string `json:"target_hints"`
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
	Detect(root string, ignorePatterns []string) ([]string, error)
}

type TechStackDetector interface {
	Detect(repoPath string) (repository.TechStackProfile, error)
}

type ServiceDetector interface {
	Detect(repo repository.Manifest) ([]string, []repository.BoundaryHint, error)
}
