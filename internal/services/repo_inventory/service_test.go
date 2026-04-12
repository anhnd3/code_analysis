package repo_inventory

import (
	"testing"

	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	scannerport "analysis-module/internal/ports/scanner"
)

func TestBuildAssignsRolesAndPlans(t *testing.T) {
	service := New()
	inventory := service.Build("ws_demo", scanFixture())
	if len(inventory.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(inventory.Repositories))
	}
	if len(inventory.Plans) != 1 {
		t.Fatalf("expected 1 extraction plan, got %d", len(inventory.Plans))
	}
	if inventory.Repositories[0].Role == repository.RoleUnknown && inventory.Repositories[1].Role == repository.RoleUnknown {
		t.Fatal("expected at least one repository role to be classified")
	}
}

func scanFixture() scannerport.ScanWorkspaceResult {
	return scannerport.ScanWorkspaceResult{
		WorkspacePath: ".",
		Repositories: []repository.Manifest{
			{
				ID:       "repo_a",
				Name:     "service-a",
				RootPath: "service-a",
				GoFiles:  []string{"main.go"},
				CandidateServices: []service.Manifest{{ID: "svc_a", Name: "service-a"}},
				TechStack:         repository.TechStackProfile{Languages: []repository.Language{repository.LanguageGo}},
			},
			{
				ID:       "repo_b",
				Name:     "docs",
				RootPath: "docs",
			},
		},
	}
}
