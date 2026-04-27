package model

type FlowTraceRequest struct {
	WorkspaceID  string `json:"workspace_id"`
	SnapshotID string `json:"snapshot_id"`
	Symbol     string `json:"symbol"`
}

type FlowPack struct {
	ID         string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
	SnapshotID     string `json:"snapshot_id"`
	RootSymbol string `json:"root_symbol"`
}

type FlowNode struct {
	ID       string `json:"id"`
	Symbol   string `json:"symbol"`
	Type       string `json:"type"`
	Status     string `json:"status"`
}

type FlowEdge struct {
	ID          string `json:"id"`
	FromNode    string `json:"from_node"`
	ToNode      string `json:"to_node"`
	Relationship string `json:"relationship"`
}