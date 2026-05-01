package sqlite

import "path/filepath"

func PathFor(artifactRoot, workspaceID, snapshotID string) string {
	return filepath.Join(filepath.Clean(artifactRoot), "workspaces", workspaceID, "snapshots", snapshotID, "facts.sqlite")
}
