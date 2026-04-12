package reviewbundle

import (
	"time"

	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/service"
	"analysis-module/internal/domain/workspace"
)

const BundleVersionV1 = "v1"

type PreviewMode string

const (
	PreviewModeEmbeddedJSON PreviewMode = "embedded_json"
	PreviewModeEmbeddedText PreviewMode = "embedded_text"
	PreviewModeListedOnly   PreviewMode = "listed_only"
)

type Snapshot struct {
	ID          string                 `json:"id"`
	WorkspaceID string                 `json:"workspace_id"`
	CreatedAt   time.Time              `json:"created_at"`
	Metadata    graph.SnapshotMetadata `json:"metadata"`
}

type Graph struct {
	Nodes          []graph.Node           `json:"nodes"`
	Edges          []graph.Edge           `json:"edges"`
	NodeKindCounts map[graph.NodeKind]int `json:"node_kind_counts"`
	EdgeKindCounts map[graph.EdgeKind]int `json:"edge_kind_counts"`
	TotalNodeCount int                    `json:"total_node_count"`
	TotalEdgeCount int                    `json:"total_edge_count"`
}

type File struct {
	ArtifactType string      `json:"artifact_type"`
	RelativePath string      `json:"relative_path"`
	ContentType  string      `json:"content_type"`
	SizeBytes    int64       `json:"size_bytes"`
	PreviewMode  PreviewMode `json:"preview_mode"`
	EmbeddedJSON any         `json:"embedded_json,omitempty"`
	EmbeddedText string      `json:"embedded_text,omitempty"`
}

type Bundle struct {
	WorkspaceID         string                        `json:"workspace_id"`
	SnapshotID          string                        `json:"snapshot_id"`
	BundleVersion       string                        `json:"bundle_version"`
	GeneratedAt         time.Time                     `json:"generated_at"`
	WorkspaceManifest   workspace.Manifest            `json:"workspace_manifest"`
	RepositoryManifests []repository.Manifest         `json:"repository_manifests"`
	ServiceManifests    []service.Manifest            `json:"service_manifests"`
	Snapshot            Snapshot                      `json:"snapshot"`
	QualityReport       quality.AnalysisQualityReport `json:"quality_report"`
	Graph               Graph                         `json:"graph"`
	Files               []File                        `json:"files"`
}
