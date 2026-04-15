package flow

// StepKind classifies the execution shape of a flow step.
type StepKind string

const (
	StepCall       StepKind = "call"
	StepConstruct  StepKind = "construct"
	StepAsync      StepKind = "async"
	StepDefer      StepKind = "defer"
	StepBranch     StepKind = "branch"
	StepBoundary   StepKind = "boundary"
)

// Step is a single unit in a stitched execution flow.
type Step struct {
	FromNodeID string   `json:"from_node_id"`
	ToNodeID   string   `json:"to_node_id"`
	EdgeID     string   `json:"edge_id,omitempty"`
	Kind       StepKind `json:"kind"`
	Label      string   `json:"label,omitempty"`
	Inferred   bool     `json:"inferred,omitempty"`
}

// Chain is an ordered sequence of Steps forming a single execution path.
type Chain struct {
	RootNodeID   string `json:"root_node_id"`
	RepositoryID string `json:"repository_id"`
	ServiceID    string `json:"service_id,omitempty"`
	Steps        []Step `json:"steps"`
}

// BoundaryMarker flags a node sitting on a protocol boundary.
type BoundaryMarker struct {
	NodeID   string `json:"node_id"`
	Protocol string `json:"protocol"`
	Role     string `json:"role"` // "client" or "server"
	Detail   string `json:"detail,omitempty"`
}

// Bundle is the complete output of a flow stitching pass.
type Bundle struct {
	Chains          []Chain          `json:"chains"`
	BoundaryMarkers []BoundaryMarker `json:"boundary_markers"`
}
