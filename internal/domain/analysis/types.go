package analysis

type SnapshotRef struct {
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id"`
}

type SnapshotFingerprint struct {
	WorkspaceID      string   `json:"workspace_id"`
	RepositoryIDs    []string `json:"repository_ids"`
	TrackedFileCount int      `json:"tracked_file_count"`
}
