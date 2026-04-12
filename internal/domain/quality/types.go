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
}
