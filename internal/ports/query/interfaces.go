package query

import "analysis-module/internal/domain/graph"

type BlastRadiusRequest struct {
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id"`
	Target      string `json:"target"`
	MaxDepth    int    `json:"max_depth"`
}

type ImpactedTestsRequest struct {
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id"`
	Target      string `json:"target"`
	MaxDepth    int    `json:"max_depth"`
}

type ImpactedEntity struct {
	Node       graph.Node       `json:"node"`
	Distance   int              `json:"distance"`
	Path       graph.Path       `json:"path"`
	Confidence graph.Confidence `json:"confidence"`
}

type BlastRadiusResult struct {
	SnapshotID string           `json:"snapshot_id"`
	Target     string           `json:"target"`
	Impacted   []ImpactedEntity `json:"impacted"`
}

type ImpactedTestsResult struct {
	SnapshotID string           `json:"snapshot_id"`
	Target     string           `json:"target"`
	Tests      []ImpactedEntity `json:"tests"`
}

type Service interface {
	BlastRadius(req BlastRadiusRequest) (BlastRadiusResult, error)
	ImpactedTests(req ImpactedTestsRequest) (ImpactedTestsResult, error)
}
