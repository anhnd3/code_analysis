package ids

import "testing"

func TestSlugPreservesSimpleIdentifiers(t *testing.T) {
	if got := Slug("software_agent_src"); got != "software_agent_src" {
		t.Fatalf("expected simple slug to stay unchanged, got %q", got)
	}
}

func TestSlugCollapsesSpacesAndSymbols(t *testing.T) {
	if got := Slug("  My Workspace!!! Repo  "); got != "my_workspace_repo" {
		t.Fatalf("expected collapsed slug, got %q", got)
	}
}

func TestSlugFallsBackWhenEmptyAfterSanitization(t *testing.T) {
	if got := Slug("___"); got != "workspace" {
		t.Fatalf("expected fallback slug, got %q", got)
	}
}
