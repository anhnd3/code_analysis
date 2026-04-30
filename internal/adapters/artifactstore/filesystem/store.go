package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"

	"analysis-module/internal/domain/artifact"
	artifactport "analysis-module/internal/ports/artifactstore"
)

type Store struct {
	root string
}

func New(root string) artifactport.Store {
	return Store{root: root}
}

func (s Store) SaveJSON(workspaceID, snapshotID, fileName string, artifactType artifact.Type, payload any) (artifact.Ref, error) {
	path := s.pathFor(workspaceID, snapshotID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return artifact.Ref{}, err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return artifact.Ref{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return artifact.Ref{}, err
	}
	return artifact.Ref{Type: artifactType, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: path}, nil
}

func (s Store) SaveText(workspaceID, snapshotID, fileName string, artifactType artifact.Type, body string) (artifact.Ref, error) {
	path := s.pathFor(workspaceID, snapshotID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return artifact.Ref{}, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return artifact.Ref{}, err
	}
	return artifact.Ref{Type: artifactType, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: path}, nil
}

func (s Store) pathFor(workspaceID, snapshotID, fileName string) string {
	if snapshotID == "" {
		return filepath.Join(s.root, "workspaces", workspaceID, fileName)
	}
	return filepath.Join(s.root, "workspaces", workspaceID, "snapshots", snapshotID, fileName)
}
