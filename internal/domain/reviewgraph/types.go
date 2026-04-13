package reviewgraph

import "time"

type NodeKind string
type NodeRole string
type EdgeType string
type FlowKind string
type ArtifactType string
type TraversalMode string

const (
	NodeFunction      NodeKind = "function"
	NodeMethod        NodeKind = "method"
	NodeClass         NodeKind = "class"
	NodeType          NodeKind = "type"
	NodeModule        NodeKind = "module"
	NodeFile          NodeKind = "file"
	NodeService       NodeKind = "service"
	NodeWorkflow      NodeKind = "workflow"
	NodeContainer     NodeKind = "container"
	NodeHTTPEndpoint  NodeKind = "http_endpoint"
	NodeGRPCMethod    NodeKind = "grpc_method"
	NodeEventTopic    NodeKind = "event_topic"
	NodePubSubChannel NodeKind = "pubsub_channel"
	NodeQueue         NodeKind = "queue"
	NodeSchedulerJob  NodeKind = "scheduler_job"
	NodeAsyncTask     NodeKind = "async_task"
	NodeInProcChannel NodeKind = "inproc_channel"

	RoleNormal        NodeRole = "normal"
	RoleEntrypoint    NodeRole = "entrypoint"
	RoleBoundary      NodeRole = "boundary"
	RolePublicAPI     NodeRole = "public_api"
	RoleAsyncProducer NodeRole = "async_producer"
	RoleAsyncConsumer NodeRole = "async_consumer"
	RoleScheduler     NodeRole = "scheduler"
	RoleSharedInfra   NodeRole = "shared_infra"

	EdgeDefines           EdgeType = "DEFINES"
	EdgeImports           EdgeType = "IMPORTS"
	EdgeCalls             EdgeType = "CALLS"
	EdgeBelongsToService  EdgeType = "BELONGS_TO_SERVICE"
	EdgeEmitsEvent        EdgeType = "EMITS_EVENT"
	EdgeConsumesEvent     EdgeType = "CONSUMES_EVENT"
	EdgePublishesMessage  EdgeType = "PUBLISHES_MESSAGE"
	EdgeSubscribesMessage EdgeType = "SUBSCRIBES_MESSAGE"
	EdgeEnqueuesJob       EdgeType = "ENQUEUES_JOB"
	EdgeDequeuesJob       EdgeType = "DEQUEUES_JOB"
	EdgeSchedulesTask     EdgeType = "SCHEDULES_TASK"
	EdgeTriggersHTTP      EdgeType = "TRIGGERS_HTTP"
	EdgeSpawnsAsync       EdgeType = "SPAWNS_ASYNC"
	EdgeRunsAsync         EdgeType = "RUNS_ASYNC"
	EdgeSendsToChannel    EdgeType = "SENDS_TO_CHANNEL"
	EdgeReceivesFromChannel EdgeType = "RECEIVES_FROM_CHANNEL"
	EdgeContains          EdgeType = "CONTAINS"
	EdgeBelongsToContainer EdgeType = "BELONGS_TO_CONTAINER"
	EdgeEntrypointFor     EdgeType = "ENTRYPOINT_FOR"
	EdgeOrchestrates      EdgeType = "ORCHESTRATES"
	EdgeUsesSharedHelper  EdgeType = "USES_SHARED_HELPER"
	EdgeCollapsesHelper   EdgeType = "COLLAPSES_HELPER"

	FlowSync  FlowKind = "sync"
	FlowAsync FlowKind = "async"

	ArtifactImportManifest  ArtifactType = "import_manifest"
	ArtifactResolvedTargets ArtifactType = "resolved_targets"
	ArtifactReviewFlow      ArtifactType = "review_flow"
	ArtifactReviewIndex     ArtifactType = "review_index"
	ArtifactReviewThreadIndex ArtifactType = "review_thread_index"
	ArtifactReviewThreadOverview ArtifactType = "review_thread_overview"
	ArtifactReviewThreadFocus ArtifactType = "review_thread_focus"
	ArtifactResidualSummary ArtifactType = "review_residuals"
	ArtifactDiagnostics     ArtifactType = "review_diagnostics"
	ArtifactRunManifest     ArtifactType = "run_manifest"
	ArtifactLayeringPlan    ArtifactType = "layering_plan"
	ArtifactLayeredTargets  ArtifactType = "layered_targets"
	ArtifactLayeringCompare ArtifactType = "layering_compare"

	TraversalFullFlow TraversalMode = "full-flow"
	TraversalBounded  TraversalMode = "bounded"
)

const (
	ImporterVersion       = "review-graph-importer/v1"
	AsyncHeuristicVersion = "async-heuristics/v1"
	AsyncV2Version        = "async-v2"
)

type Node struct {
	ID           string   `json:"id"`
	SnapshotID   string   `json:"snapshot_id"`
	Repo         string   `json:"repo"`
	Service      string   `json:"service"`
	Language     string   `json:"language"`
	Kind         NodeKind `json:"kind"`
	Symbol       string   `json:"symbol"`
	FilePath     string   `json:"file_path"`
	StartLine    int      `json:"start_line,omitempty"`
	EndLine      int      `json:"end_line,omitempty"`
	Signature    string   `json:"signature,omitempty"`
	Visibility   string   `json:"visibility,omitempty"`
	NodeRole     NodeRole `json:"node_role,omitempty"`
	MetadataJSON string   `json:"metadata_json,omitempty"`
}

type Edge struct {
	ID             string   `json:"id"`
	SnapshotID     string   `json:"snapshot_id"`
	SrcID          string   `json:"src_id"`
	DstID          string   `json:"dst_id"`
	EdgeType       EdgeType `json:"edge_type"`
	FlowKind       FlowKind `json:"flow_kind"`
	Confidence     float64  `json:"confidence,omitempty"`
	EvidenceFile   string   `json:"evidence_file,omitempty"`
	EvidenceLine   int      `json:"evidence_line,omitempty"`
	EvidenceText   string   `json:"evidence_text,omitempty"`
	Transport      string   `json:"transport,omitempty"`
	TopicOrChannel string   `json:"topic_or_channel,omitempty"`
	MetadataJSON   string   `json:"metadata_json,omitempty"`
}

type Artifact struct {
	ID           string       `json:"id"`
	SnapshotID   string       `json:"snapshot_id"`
	ArtifactType ArtifactType `json:"artifact_type"`
	TargetNodeID string       `json:"target_node_id,omitempty"`
	Path         string       `json:"path"`
	MetadataJSON string       `json:"metadata_json,omitempty"`
}

type ResolvedTarget struct {
	TargetNodeID string         `json:"target_node_id"`
	DisplayName  string         `json:"display_name"`
	Reason       string         `json:"reason"`
	SourceInput  string         `json:"source_input"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type CycleSummary struct {
	Path               []string `json:"path"`
	CrossService       bool     `json:"cross_service"`
	CrossAsyncBoundary bool     `json:"cross_async_boundary"`
}

type TraversalCaps struct {
	MaxNodes                int `json:"max_nodes"`
	MaxEdges                int `json:"max_edges"`
	MaxPaths                int `json:"max_paths"`
	MaxAsyncFanoutPerBridge int `json:"max_async_fanout_per_bridge"`
}

func DefaultTraversalCaps() TraversalCaps {
	return TraversalCaps{
		MaxNodes:                500,
		MaxEdges:                1000,
		MaxPaths:                200,
		MaxAsyncFanoutPerBridge: 25,
	}
}

type PathSummary struct {
	NodeIDs        []string `json:"node_ids"`
	TerminalReason string   `json:"terminal_reason"`
	Direction      string   `json:"direction"`
	Truncated      bool     `json:"truncated,omitempty"`
}

type AsyncParticipant struct {
	NodeID      string `json:"node_id"`
	Service     string `json:"service,omitempty"`
	DisplayName string `json:"display_name"`
}

type AsyncBridgeSummary struct {
	BridgeNodeID        string             `json:"bridge_node_id"`
	BridgeDisplayName   string             `json:"bridge_display_name"`
	BridgeKind          NodeKind           `json:"bridge_kind"`
	Transport           string             `json:"transport,omitempty"`
	TopicOrChannel      string             `json:"topic_or_channel,omitempty"`
	ProducerCount       int                `json:"producer_count"`
	ConsumerCount       int                `json:"consumer_count"`
	Producers           []AsyncParticipant `json:"producers,omitempty"`
	Consumers           []AsyncParticipant `json:"consumers,omitempty"`
	UpstreamSyncPaths   []PathSummary      `json:"upstream_sync_paths,omitempty"`
	DownstreamSyncPaths []PathSummary      `json:"downstream_sync_paths,omitempty"`
	FanoutTruncated     bool               `json:"fanout_truncated,omitempty"`
}

type CoverageStats struct {
	CoveredNodeCount  int `json:"covered_node_count"`
	CoveredEdgeCount  int `json:"covered_edge_count"`
	SharedInfraCount  int `json:"shared_infra_count"`
	ResidualNodeCount int `json:"residual_node_count,omitempty"`
}

type TraversalResult struct {
	TargetNodeID        string               `json:"target_node_id"`
	Mode                TraversalMode        `json:"mode"`
	SyncUpstreamPaths   []PathSummary        `json:"sync_upstream_paths"`
	SyncDownstreamPaths []PathSummary        `json:"sync_downstream_paths"`
	AsyncBridges        []AsyncBridgeSummary `json:"async_bridges,omitempty"`
	CoveredNodeIDs      []string             `json:"covered_node_ids"`
	CoveredEdgeIDs      []string             `json:"covered_edge_ids"`
	AffectedFiles       []string             `json:"affected_files"`
	CrossServices       []string             `json:"cross_services"`
	Ambiguities         []string             `json:"ambiguities"`
	Cycles              []CycleSummary       `json:"cycles"`
	TruncationWarnings  []string             `json:"truncation_warnings"`
	Coverage            CoverageStats        `json:"coverage"`
}

type ImportDiagnostic struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	FilePath string `json:"file_path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type ImportCounts struct {
	LegacyNodeCount         int `json:"legacy_node_count"`
	LegacyEdgeCount         int `json:"legacy_edge_count"`
	ImportedNodeCount       int `json:"imported_node_count"`
	ImportedEdgeCount       int `json:"imported_edge_count"`
	DroppedTestNodeCount    int `json:"dropped_test_node_count"`
	DroppedTestEdgeCount    int `json:"dropped_test_edge_count"`
	DroppedIgnoredNodes     int `json:"dropped_ignored_nodes"`
	DroppedIgnoredEdges     int `json:"dropped_ignored_edges"`
	DroppedGeneratedNodes   int `json:"dropped_generated_nodes"`
	DroppedGeneratedEdges   int `json:"dropped_generated_edges"`
	WeakAsyncMatches        int `json:"weak_async_matches"`
	ContextualBoundaryEdges int `json:"contextual_boundary_edges"`
}

type ImportManifest struct {
	WorkspaceID     string             `json:"workspace_id"`
	SnapshotID      string             `json:"snapshot_id"`
	ImporterVersion string             `json:"importer_version"`
	GeneratedAt     time.Time          `json:"generated_at"`
	InputPaths      map[string]string  `json:"input_paths"`
	IgnoreFiles     []string           `json:"ignore_files"`
	IgnoreRules     []string           `json:"ignore_rules"`
	Counts          ImportCounts       `json:"counts"`
	Diagnostics     []ImportDiagnostic `json:"diagnostics,omitempty"`
	AsyncVersion    string             `json:"async_heuristic_version"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
}

type RunManifest struct {
	WorkspaceID     string            `json:"workspace_id"`
	SnapshotID      string            `json:"snapshot_id"`
	ImporterVersion string            `json:"importer_version"`
	AsyncVersion    string            `json:"async_heuristic_version"`
	GeneratedAt     time.Time         `json:"generated_at"`
	InputPaths      map[string]string `json:"input_paths"`
	IgnoreFiles     []string          `json:"ignore_files"`
	IgnoreRules     []string          `json:"ignore_rules"`
	DroppedCounts   ImportCounts      `json:"dropped_counts"`
	TraversalDefaults struct {
		Mode         TraversalMode `json:"mode"`
		IncludeAsync bool          `json:"include_async"`
		Caps         TraversalCaps `json:"caps"`
		ForwardDepth int           `json:"forward_depth,omitempty"`
		ReverseDepth int           `json:"reverse_depth,omitempty"`
	} `json:"traversal_defaults"`
	TargetFile string `json:"target_file"`
}

func IsStructuralEdge(edgeType EdgeType) bool {
	switch edgeType {
	case EdgeDefines, EdgeImports, EdgeBelongsToService:
		return true
	default:
		return false
	}
}

func IsAsyncEdge(edgeType EdgeType) bool {
	switch edgeType {
	case EdgeEmitsEvent, EdgeConsumesEvent, EdgePublishesMessage, EdgeSubscribesMessage, EdgeEnqueuesJob, EdgeDequeuesJob, EdgeSchedulesTask, EdgeTriggersHTTP, EdgeSpawnsAsync, EdgeRunsAsync, EdgeSendsToChannel, EdgeReceivesFromChannel:
		return true
	default:
		return false
	}
}

func IsCoverageEdge(edgeType EdgeType) bool {
	return edgeType == EdgeCalls || IsAsyncEdge(edgeType)
}
