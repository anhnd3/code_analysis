package artifact

type Type string

const (
	TypeWorkspaceManifest     Type = "workspace_manifest"
	TypeRepositoryManifests   Type = "repository_manifests"
	TypeServiceManifests      Type = "service_manifests"
	TypeScanWarnings          Type = "scan_warnings"
	TypeGraphNodes            Type = "graph_nodes"
	TypeGraphEdges            Type = "graph_edges"
	TypeQualityReport         Type = "quality_report"
	TypeBuildSnapshotResult   Type = "build_snapshot_result"
	TypeReviewBundle          Type = "review_bundle"
	TypeBlastRadius           Type = "blast_radius"
	TypeImpactedTests         Type = "impacted_tests"
	TypeFlowBundle            Type = "flow_bundle"
	TypeBoundaryBundle        Type = "boundary_bundle"
	TypeRootExports           Type = "root_exports"
	TypeReducedChain          Type = "reduced_chain"
	TypeReviewFlow            Type = "review_flow"
	TypeReviewFlowBuild       Type = "review_flow_build"
	TypeSequenceModel         Type = "sequence_model"
	TypeMermaidDiagram        Type = "mermaid_diagram"
	TypeServiceCoverageReport Type = "service_coverage_report"
	TypeServiceReviewPack     Type = "service_review_pack"
	TypeSelectedFlows         Type = "selected_flows"
)

type Ref struct {
	Type        Type   `json:"type"`
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id,omitempty"`
	Path        string `json:"path"`
}
