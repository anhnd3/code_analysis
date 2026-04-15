package quality

import "analysis-module/internal/domain/analysis"

type CoverageMetric struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Total int    `json:"total"`
}

type GapReport struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Count    int    `json:"count"`
}

type AnalysisQualityReport struct {
	SnapshotID  string               `json:"snapshot_id"`
	Metrics     []CoverageMetric     `json:"metrics"`
	IssueCounts analysis.IssueCounts `json:"issue_counts"`
	Gaps        []GapReport          `json:"gaps"`
	FlowMetrics *FlowQualityMetrics  `json:"flow_metrics,omitempty"`
}

type FlowQualityMetrics struct {
	ResolvedEntrypoints    int `json:"resolved_entrypoints"`
	StitchedEdges          int `json:"stitched_edges"`
	BoundaryMarkers        int `json:"boundary_markers"`
	ConfirmedLinks         int `json:"confirmed_links"`
	SubsetCompatibleLinks  int `json:"subset_compatible_links"`
	CandidateLinks         int `json:"candidate_links"`
	MismatchLinks          int `json:"mismatch_links"`
	ExternalOnlyLinks      int `json:"external_only_links"`
	ReducedChainsGenerated int `json:"reduced_chains_generated"`
	MermaidExportsGenerated int `json:"mermaid_exports_generated"`
}
