package snapshot_manage

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"analysis-module/pkg/ids"
)

func TestNewWorkspaceIDUsesWorkspaceBaseName(t *testing.T) {
	root := filepath.Join("/tmp", "software_agent_src")

	got := New().NewWorkspaceID(root)
	want := "ws_software_agent_src_" + ids.StableSuffix(filepath.Clean(root))
	if got != want {
		t.Fatalf("expected workspace id %q, got %q", want, got)
	}
}

func TestNewWorkspaceIDCollapsesSymbolsInBaseName(t *testing.T) {
	root := filepath.Join("/tmp", "My Workspace!!!")

	got := New().NewWorkspaceID(root)
	want := "ws_my_workspace_" + ids.StableSuffix(filepath.Clean(root))
	if got != want {
		t.Fatalf("expected workspace id %q, got %q", want, got)
	}
}

func TestNewWorkspaceIDFallsBackWhenBaseNameHasNoSlugCharacters(t *testing.T) {
	root := filepath.Join("/tmp", "___")

	got := New().NewWorkspaceID(root)
	want := "ws_workspace_" + ids.StableSuffix(filepath.Clean(root))
	if got != want {
		t.Fatalf("expected workspace id %q, got %q", want, got)
	}
}

func TestNewSnapshotIDUsesFriendlyWorkspacePrefix(t *testing.T) {
	workspaceID := "ws_software_agent_src_42d2f398e016"

	got := New().NewSnapshotID(workspaceID)
	if !strings.HasPrefix(got, workspaceID+"_") {
		t.Fatalf("expected snapshot id %q to start with workspace id", got)
	}
	if !regexp.MustCompile(`^ws_software_agent_src_42d2f398e016_\d{8}T\d{6}Z$`).MatchString(got) {
		t.Fatalf("unexpected snapshot id format: %q", got)
	}
}
