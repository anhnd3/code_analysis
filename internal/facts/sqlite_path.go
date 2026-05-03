package facts

import "path/filepath"

func SQLitePathFor(artifactRoot, workspaceID, snapshotID string) string {
	return filepath.Join(filepath.Clean(artifactRoot), "workspaces", workspaceID, "snapshots", snapshotID, "facts.sqlite")
}
