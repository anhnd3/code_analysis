package quality

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
	SnapshotID string           `json:"snapshot_id"`
	Metrics    []CoverageMetric `json:"metrics"`
	Gaps       []GapReport      `json:"gaps"`
}
