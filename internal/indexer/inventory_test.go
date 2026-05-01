package indexer

import (
	"testing"

	"analysis-module/internal/domain/repository"
	scannerport "analysis-module/internal/ports/scanner"
)

func TestInventoryBuilderBuildsBasic(t *testing.T) {
	builder := NewInventoryBuilder()

	scanResult := scannerport.ScanWorkspaceResult{
		Repositories: []repository.Manifest{
			{
				ID:       "repo-1",
				Name:     "test-repo",
				RootPath: "/tmp/test",
				TechStack: repository.TechStackProfile{
					Languages: []repository.Language{repository.LanguageGo},
				},
				GoFiles: []string{"main.go"},
			},
		},
	}

	inventory := builder.Build("ws-1", scanResult)
	if len(inventory.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(inventory.Repositories))
	}
	if inventory.Repositories[0].Role != repository.RoleService && inventory.Repositories[0].Role != repository.RoleSharedLib && inventory.Repositories[0].Role != repository.RoleUnknown {
		t.Logf("role: %s", inventory.Repositories[0].Role)
	}

	if len(inventory.Plans) != 1 {
		t.Fatalf("expected 1 extraction plan, got %d", len(inventory.Plans))
	}
	if inventory.Plans[0].Language != repository.LanguageGo {
		t.Errorf("expected Go language in plan, got %s", inventory.Plans[0].Language)
	}
}

func TestInventoryBuilderBuildsWithMultipleLanguages(t *testing.T) {
	builder := NewInventoryBuilder()

	scanResult := scannerport.ScanWorkspaceResult{
		Repositories: []repository.Manifest{
			{
				ID:       "repo-1",
				Name:     "multi-lang",
				RootPath: "/tmp/multi",
				TechStack: repository.TechStackProfile{
					Languages: []repository.Language{repository.LanguageGo, repository.LanguagePython},
				},
				GoFiles:     []string{"main.go"},
				PythonFiles: []string{"app.py"},
			},
		},
	}

	inventory := builder.Build("ws-2", scanResult)
	if len(inventory.Plans) != 2 {
		t.Fatalf("expected 2 extraction plans, got %d", len(inventory.Plans))
	}
}
