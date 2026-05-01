package flow

type FlowTraceRequest struct {
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id"`
	Symbol      string `json:"symbol"`
}

type FlowPack struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	SnapshotID  string           `json:"snapshot_id"`
	Root        FlowNode         `json:"root"`
	Nodes       []FlowNode       `json:"nodes"`
	Edges       []FlowEdge       `json:"edges"`
	Evidence    []EvidenceItem   `json:"evidence"`
	Ambiguities []AmbiguityItem  `json:"ambiguities"`
	Diagnostics []DiagnosticItem `json:"diagnostics"`
}

type FlowNode struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status,omitempty"`

	SymbolID      string `json:"symbol_id,omitempty"`
	Symbol        string `json:"symbol,omitempty"`
	CanonicalName string `json:"canonical_name,omitempty"`

	ServiceID    string `json:"service_id,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
}

type FlowEdge struct {
	ID          string   `json:"id"`
	FromNodeID  string   `json:"from_node_id"`
	ToNodeID    string   `json:"to_node_id"`
	Type        string   `json:"type"`
	Status      string   `json:"status"`
	Confidence  float64  `json:"confidence,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Rationale   string   `json:"rationale,omitempty"`
}

type EvidenceItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	FilePath  string `json:"file_path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Query     string `json:"query,omitempty"`
	Source    string `json:"source,omitempty"`
}

type AmbiguityItem struct {
	ID       string `json:"id"`
	EdgeID   string `json:"edge_id,omitempty"`
	Reason   string `json:"reason"`
	Evidence string `json:"evidence,omitempty"`
}

type DiagnosticItem struct {
	ID       string `json:"id"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
}
