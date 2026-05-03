package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"analysis-module/internal/indexer"
)

type Store struct {
	root string
}

func NewArtifactStore(root string) indexer.ArtifactStore {
	return Store{root: root}
}

func (s Store) SaveJSON(workspaceID, snapshotID, fileName string, artifactType indexer.ArtifactType, payload any) (indexer.ArtifactRef, error) {
	path := s.pathFor(workspaceID, snapshotID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return indexer.ArtifactRef{}, err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return indexer.ArtifactRef{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return indexer.ArtifactRef{}, err
	}
	return indexer.ArtifactRef{Type: artifactType, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: path}, nil
}

func (s Store) SaveText(workspaceID, snapshotID, fileName string, artifactType indexer.ArtifactType, body string) (indexer.ArtifactRef, error) {
	path := s.pathFor(workspaceID, snapshotID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return indexer.ArtifactRef{}, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return indexer.ArtifactRef{}, err
	}
	return indexer.ArtifactRef{Type: artifactType, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: path}, nil
}

func (s Store) pathFor(workspaceID, snapshotID, fileName string) string {
	if snapshotID == "" {
		return filepath.Join(s.root, "workspaces", workspaceID, fileName)
	}
	return filepath.Join(s.root, "workspaces", workspaceID, "snapshots", snapshotID, fileName)
}
