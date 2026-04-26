package scan

import (
	"path/filepath"

	"analysis-module/internal/domain/repository"
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
	WorkspaceID string               `json:"workspace_id"`
	Inventory   repository.Inventory `json:"inventory"`
	Warnings    []string             `json:"warnings"`
}

type Service struct {
	scanner        workspace_scan.Service
	inventory      repo_inventory.Service
	snapshotManage snapshot_manage.Service
}

func New(scanner workspace_scan.Service, inventory repo_inventory.Service, snapshotManage snapshot_manage.Service) Service {
	return Service{
		scanner:        scanner,
		inventory:      inventory,
		snapshotManage: snapshotManage,
	}
}

func (s Service) Run(req Request) (Result, error) {
	workspacePath := filepath.Clean(req.WorkspacePath)
	workspaceID := s.snapshotManage.NewWorkspaceID(workspacePath)
	scanResult, err := s.scanner.Scan(scannerport.ScanWorkspaceRequest{
		WorkspacePath:  workspacePath,
		IgnorePatterns: req.IgnorePatterns,
		TargetHints:    req.TargetHints,
	})
	if err != nil {
		return Result{}, err
	}
	inventory := s.inventory.Build(workspaceID, scanResult)
	return Result{
		WorkspaceID: workspaceID,
		Inventory:   inventory,
		Warnings:    append([]string{}, scanResult.Warnings...),
	}, nil
}
