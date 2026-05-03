package indexer

import (
	"testing"
)

func TestInventoryBuilderBuildsBasic(t *testing.T) {
	builder := NewInventoryBuilder()

	scanResult := ScanWorkspaceResult{
		Repositories: []Manifest{
			{
				ID:       "repo-1",
				Name:     "test-repo",
				RootPath: "/tmp/test",
				TechStack: TechStackProfile{
					Languages: []Language{LanguageGo},
				},
				GoFiles: []string{"main.go"},
			},
		},
	}

	inventory := builder.Build("ws-1", scanResult)
	if len(inventory.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(inventory.Repositories))
	}
	if inventory.Repositories[0].Role != RoleService && inventory.Repositories[0].Role != RoleSharedLib && inventory.Repositories[0].Role != RoleUnknown {
		t.Logf("role: %s", inventory.Repositories[0].Role)
	}

	if len(inventory.Plans) != 1 {
		t.Fatalf("expected 1 extraction plan, got %d", len(inventory.Plans))
	}
	if inventory.Plans[0].Language != LanguageGo {
		t.Errorf("expected Go language in plan, got %s", inventory.Plans[0].Language)
	}
}

func TestInventoryBuilderBuildsWithMultipleLanguages(t *testing.T) {
	builder := NewInventoryBuilder()

	scanResult := ScanWorkspaceResult{
		Repositories: []Manifest{
			{
				ID:       "repo-1",
				Name:     "multi-lang",
				RootPath: "/tmp/multi",
				TechStack: TechStackProfile{
					Languages: []Language{LanguageGo, LanguagePython},
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
