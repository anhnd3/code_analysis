package reviewgraph_paths

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ResolvedPaths struct {
	WorkspaceManifestPath  string
	RepositoryManifestPath string
	ServiceManifestPath    string
	QualityReportPath      string
	NodesPath              string
	EdgesPath              string
	ReviewGraphDBPath      string
	ReviewDir              string
	ResolvedTargetsPath    string
}

type Service struct {
	artifactRoot string
}

func New(artifactRoot string) Service {
	return Service{artifactRoot: filepath.Clean(artifactRoot)}
}

func (s Service) Resolve(workspaceID, snapshotID string) (ResolvedPaths, error) {
	if workspaceID == "" {
		return ResolvedPaths{}, fmt.Errorf("workspace id is required")
	}
	if snapshotID == "" {
		return ResolvedPaths{}, fmt.Errorf("snapshot id is required")
	}
	base := filepath.Join(s.artifactRoot, "workspaces", workspaceID)
	snapshotBase := filepath.Join(base, "snapshots", snapshotID)
	reviewDir := filepath.Join(snapshotBase, "review")
	return ResolvedPaths{
		WorkspaceManifestPath:  filepath.Join(base, "workspace_manifest.json"),
		RepositoryManifestPath: filepath.Join(base, "repository_manifests.json"),
		ServiceManifestPath:    filepath.Join(base, "service_manifests.json"),
		QualityReportPath:      filepath.Join(snapshotBase, "quality_report.json"),
		NodesPath:              filepath.Join(snapshotBase, "graph_nodes.jsonl"),
		EdgesPath:              filepath.Join(snapshotBase, "graph_edges.jsonl"),
		ReviewGraphDBPath:      filepath.Join(snapshotBase, "sqlite", "review_graph.sqlite"),
		ReviewDir:              reviewDir,
		ResolvedTargetsPath:    filepath.Join(reviewDir, "resolved_targets.json"),
	}, nil
}

func (s Service) ReviewDirFromDBPath(dbPath string) string {
	clean := filepath.Clean(dbPath)
	parent := filepath.Dir(clean)
	if strings.EqualFold(filepath.Base(parent), "sqlite") {
		return filepath.Join(filepath.Dir(parent), "review")
	}
	return filepath.Join(filepath.Dir(clean), "review")
}
