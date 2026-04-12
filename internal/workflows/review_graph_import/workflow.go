package review_graph_import

import "analysis-module/internal/services/reviewgraph_import"

type Request struct {
	WorkspaceID         string `json:"workspace_id"`
	SnapshotID          string `json:"snapshot_id"`
	NodesPath           string `json:"nodes_path,omitempty"`
	EdgesPath           string `json:"edges_path,omitempty"`
	RepoManifestPath    string `json:"repo_manifest_path,omitempty"`
	ServiceManifestPath string `json:"service_manifest_path,omitempty"`
	QualityReportPath   string `json:"quality_report_path,omitempty"`
	IgnoreFilePath      string `json:"ignore_file_path,omitempty"`
	OutDBPath           string `json:"out_db_path,omitempty"`
}

type Result = reviewgraph_import.Result

type Workflow struct {
	service reviewgraph_import.Service
}

func New(service reviewgraph_import.Service) Workflow {
	return Workflow{service: service}
}

func (w Workflow) Run(req Request) (Result, error) {
	return w.service.Import(reviewgraph_import.Request(req))
}
