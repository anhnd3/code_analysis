package filesystem

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
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

func (s Store) SaveGraph(workspaceID, snapshotID string, nodes []graph.Node, edges []graph.Edge) ([]artifact.Ref, error) {
	nodePath := s.pathFor(workspaceID, snapshotID, "graph_nodes.jsonl")
	edgePath := s.pathFor(workspaceID, snapshotID, "graph_edges.jsonl")
	if err := os.MkdirAll(filepath.Dir(nodePath), 0o755); err != nil {
		return nil, err
	}
	if err := writeJSONL(nodePath, nodes); err != nil {
		return nil, err
	}
	if err := writeJSONL(edgePath, edges); err != nil {
		return nil, err
	}
	return []artifact.Ref{
		{Type: artifact.TypeGraphNodes, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: nodePath},
		{Type: artifact.TypeGraphEdges, WorkspaceID: workspaceID, SnapshotID: snapshotID, Path: edgePath},
	}, nil
}

func (s Store) SaveQualityReport(workspaceID, snapshotID string, report quality.AnalysisQualityReport) (artifact.Ref, error) {
	return s.SaveJSON(workspaceID, snapshotID, "quality_report.json", artifact.TypeQualityReport, report)
}

func (s Store) pathFor(workspaceID, snapshotID, fileName string) string {
	if snapshotID == "" {
		return filepath.Join(s.root, "workspaces", workspaceID, fileName)
	}
	return filepath.Join(s.root, "workspaces", workspaceID, "snapshots", snapshotID, fileName)
}

func writeJSONL[T any](path string, items []T) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	encoder := json.NewEncoder(writer)
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}
