package reviewgraph

type DerivedNode struct {
	ID             string   `json:"id"`
	SnapshotID     string   `json:"snapshot_id"`
	AnchorTargetID string   `json:"anchor_target_id"`
	Kind           NodeKind `json:"kind"`
	Label          string   `json:"label"`
	MetadataJSON   string   `json:"metadata_json"`
}

type DerivedEdge struct {
	ID             string   `json:"id"`
	SnapshotID     string   `json:"snapshot_id"`
	AnchorTargetID string   `json:"anchor_target_id"`
	SrcID          string   `json:"src_id"`
	DstID          string   `json:"dst_id"`
	EdgeType       EdgeType `json:"edge_type"`
	MetadataJSON   string   `json:"metadata_json"`
}

type LayeringCandidateKind string
type LayeringDecision string
type LayeringConfidence string

const (
	LayeringCandidateWorkflow       LayeringCandidateKind = "workflow"
	LayeringCandidateContainer      LayeringCandidateKind = "container"
	LayeringCandidateHelperCollapse LayeringCandidateKind = "helper_collapse"

	LayeringDecisionPending  LayeringDecision = "pending"
	LayeringDecisionApproved LayeringDecision = "approved"
	LayeringDecisionRejected LayeringDecision = "rejected"

	LayeringConfidenceHigh   LayeringConfidence = "high"
	LayeringConfidenceMedium LayeringConfidence = "medium"
	LayeringConfidenceLow    LayeringConfidence = "low"
)

type SourceSpan struct {
	FilePath    string `json:"file_path"`
	SymbolID    string `json:"symbol_id,omitempty"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Reason      string `json:"reason"`
	Fingerprint string `json:"fingerprint"`
}

type LayeringEvidence struct {
	Kind    string   `json:"kind"`
	Message string   `json:"message"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

type LayeringSymbolRef struct {
	NodeID       string `json:"node_id"`
	DisplayName  string `json:"display_name"`
	FilePath     string `json:"file_path,omitempty"`
	Kind         string `json:"kind,omitempty"`
	TerminalOnly bool   `json:"terminal_only,omitempty"`
}

type LayeringHelperRef struct {
	NodeID         string `json:"node_id"`
	DisplayName    string `json:"display_name"`
	FilePath       string `json:"file_path,omitempty"`
	CrossFlowCount int    `json:"cross_flow_count,omitempty"`
	SharedInfra    bool   `json:"shared_infra,omitempty"`
}

type LayeringContainerRef struct {
	Label     string   `json:"label"`
	Subtype   string   `json:"subtype"`
	SymbolIDs []string `json:"symbol_ids"`
}

type LayeringAsyncBridgeRef struct {
	BridgeNodeID   string   `json:"bridge_node_id"`
	DisplayName    string   `json:"display_name"`
	Kind           NodeKind `json:"kind"`
	Transport      string   `json:"transport,omitempty"`
	TopicOrChannel string   `json:"topic_or_channel,omitempty"`
}

type LayeringAnchor struct {
	TargetNodeID string `json:"target_node_id"`
	DisplayName  string `json:"display_name"`
	Reason       string `json:"reason"`
	FlowFile     string `json:"flow_file,omitempty"`
}

type LayeringPlanningPacket struct {
	CandidateWorkflowRoot string                  `json:"candidate_workflow_root,omitempty"`
	CandidateContainers   []LayeringContainerRef  `json:"candidate_containers,omitempty"`
	CoreSymbols           []LayeringSymbolRef     `json:"core_symbols,omitempty"`
	SharedHelpers         []LayeringHelperRef     `json:"shared_helpers,omitempty"`
	AffectedFiles         []string                `json:"affected_files,omitempty"`
	AsyncBridges          []LayeringAsyncBridgeRef `json:"async_bridges,omitempty"`
	Diagnostics           []string                `json:"diagnostics,omitempty"`
	RecommendedSourceSpans []SourceSpan          `json:"recommended_source_spans,omitempty"`
}

type LayeringCandidate struct {
	ID              string               `json:"id"`
	CandidateKind   LayeringCandidateKind `json:"candidate_kind"`
	Label           string               `json:"label"`
	Decision        LayeringDecision     `json:"decision"`
	Confidence      LayeringConfidence   `json:"confidence"`
	Evidence        []LayeringEvidence   `json:"evidence,omitempty"`
	SourceSpans     []SourceSpan         `json:"source_spans,omitempty"`
	Fingerprint     string               `json:"fingerprint"`
	ApprovalReason  string               `json:"approval_reason,omitempty"`
	RejectionReason string               `json:"rejection_reason,omitempty"`
}

type LayeringPlan struct {
	PlanVersion   string                `json:"plan_version"`
	SnapshotID    string                `json:"snapshot_id"`
	DBPath        string                `json:"db_path"`
	TargetsFile   string                `json:"targets_file"`
	ReviewDir     string                `json:"review_dir"`
	LayeringDir   string                `json:"layering_dir"`
	Anchor        LayeringAnchor        `json:"anchor"`
	PlanningPacket LayeringPlanningPacket `json:"planning_packet"`
	Candidates    []LayeringCandidate   `json:"candidates"`
}

type DerivedNodeMetadata struct {
	CandidateID        string            `json:"candidate_id"`
	Subtype            string            `json:"subtype,omitempty"`
	Evidence           []LayeringEvidence `json:"evidence,omitempty"`
	SourceFingerprints []string          `json:"source_fingerprints,omitempty"`
	PlanPath           string            `json:"plan_path"`
	ApprovedAt         string            `json:"approved_at"`
}

type DerivedEdgeMetadata struct {
	CandidateID        string            `json:"candidate_id"`
	Subtype            string            `json:"subtype,omitempty"`
	Evidence           []LayeringEvidence `json:"evidence,omitempty"`
	SourceFingerprints []string          `json:"source_fingerprints,omitempty"`
	PlanPath           string            `json:"plan_path"`
	ApprovedAt         string            `json:"approved_at"`
}

type LayeringCompareMetrics struct {
	TopLevelTargetsBefore   int `json:"top_level_targets_before"`
	TopLevelTargetsAfter    int `json:"top_level_targets_after"`
	HelperOnlyBefore        int `json:"helper_only_before"`
	HelperOnlyAfter         int `json:"helper_only_after"`
	VisibleRawSymbolsBefore int `json:"visible_raw_symbols_before"`
	VisibleRawSymbolsAfter  int `json:"visible_raw_symbols_after"`
	GroupedHelperCount      int `json:"grouped_helper_count"`
	GroupedDerivedCount     int `json:"grouped_derived_count"`
	CycleLinesBefore        int `json:"cycle_lines_before"`
	CycleLinesAfter         int `json:"cycle_lines_after"`
	AffectedFilesBefore     int `json:"affected_files_before"`
	AffectedFilesAfter      int `json:"affected_files_after"`
}

