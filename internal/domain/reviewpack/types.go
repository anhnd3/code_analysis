package reviewpack

// ExpectedRoot describes one manifest-declared flow target.
type ExpectedRoot struct {
	ID         string `json:"id"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	RootType   string `json:"root_type"`
	Family     string `json:"family"`
	Required   bool   `json:"required"`
	CuratedMMD string `json:"curated_mmd,omitempty"`
}

type CoverageStatus string

const (
	CoverageRendered CoverageStatus = "rendered"
	CoverageSkipped  CoverageStatus = "skipped"
	CoverageMissing  CoverageStatus = "missing"
)

type FailureStage string

const (
	FailureStageDetection  FailureStage = "detection"
	FailureStageResolution FailureStage = "resolution"
	FailureStageStitching  FailureStage = "stitching"
	FailureStageReduction  FailureStage = "reduction"
	FailureStageRendering  FailureStage = "rendering"
	FailureStageSelection  FailureStage = "selection"
)

type DeterministicReason string

const (
	ReasonRootNotDetected         DeterministicReason = "root_not_detected"
	ReasonRootNotResolved         DeterministicReason = "root_not_resolved"
	ReasonNoStitchedChain         DeterministicReason = "no_stitched_chain"
	ReasonReductionEmpty          DeterministicReason = "reduction_empty"
	ReasonReviewRenderFailed      DeterministicReason = "review_render_failed"
	ReasonBootstrapInsufficient   DeterministicReason = "bootstrap_lifecycle_insufficient"
	ReasonUnsupportedRootType     DeterministicReason = "unsupported_root_type"
	ReasonOptionalRootNotRendered DeterministicReason = "optional_root_not_rendered"
)

type RenderSource string

const (
	RenderSourceReviewFlow         RenderSource = "reviewflow"
	RenderSourceBootstrapLifecycle RenderSource = "bootstrap_lifecycle"
	RenderSourceReducedDebug       RenderSource = "reduced_debug"
)

type PolicySource string

const (
	PolicySourceManifest      PolicySource = "manifest"
	PolicySourceRouteMetadata PolicySource = "route_metadata"
	PolicySourceDefault       PolicySource = "default"
)

type CoverageItem struct {
	ExpectedRootID   string              `json:"expected_root_id"`
	Method           string              `json:"method,omitempty"`
	Path             string              `json:"path,omitempty"`
	RootType         string              `json:"root_type"`
	Family           string              `json:"family,omitempty"`
	Required         bool                `json:"required"`
	RequiredBlocking bool                `json:"required_blocking"`
	Status           CoverageStatus      `json:"status"`
	Reason           DeterministicReason `json:"reason,omitempty"`
	FailureStage     FailureStage        `json:"failure_stage,omitempty"`
	RenderSource     RenderSource        `json:"render_source,omitempty"`
	RootNodeID       string              `json:"root_node_id,omitempty"`
	ArtifactSlug     string              `json:"artifact_slug,omitempty"`
}

type SelectedFlow struct {
	ExpectedRootID    string       `json:"expected_root_id"`
	RootNodeID        string       `json:"root_node_id"`
	CanonicalName     string       `json:"canonical_name"`
	Family            string       `json:"family,omitempty"`
	PolicyFamily      string       `json:"policy_family,omitempty"`
	PolicySource      PolicySource `json:"policy_source,omitempty"`
	RenderSource      RenderSource `json:"render_source"`
	CandidateKind     string       `json:"candidate_kind,omitempty"`
	Signature         string       `json:"signature,omitempty"`
	ParticipantCount  int          `json:"participant_count,omitempty"`
	StageCount        int          `json:"stage_count,omitempty"`
	MessageCount      int          `json:"message_count,omitempty"`
	QualityFlags      []string     `json:"quality_flags,omitempty"`
	ArtifactSlug      string       `json:"artifact_slug,omitempty"`
	MermaidPath       string       `json:"mermaid_path,omitempty"`
	ReviewFlowPath    string       `json:"review_flow_path,omitempty"`
	SequenceModelPath string       `json:"sequence_model_path,omitempty"`
}

type ServiceReviewPack struct {
	ServiceName   string         `json:"service_name"`
	ExpectedRoots []ExpectedRoot `json:"expected_roots"`
	Coverage      []CoverageItem `json:"coverage"`
	SelectedFlows []SelectedFlow `json:"selected_flows"`
}
