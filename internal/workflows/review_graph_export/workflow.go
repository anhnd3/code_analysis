package review_graph_export

import "analysis-module/internal/services/reviewgraph_export"

type Request struct {
	DBPath        string `json:"db_path"`
	TargetsFile   string `json:"targets_file"`
	Mode          string `json:"mode,omitempty"`
	RenderMode    string `json:"render_mode,omitempty"`
	CompanionView string `json:"companion_view,omitempty"`
	IncludeAsync  bool   `json:"include_async"`
	ForwardDepth  int    `json:"forward_depth,omitempty"`
	ReverseDepth  int    `json:"reverse_depth,omitempty"`
	OutDir        string `json:"out_dir,omitempty"`
}

type Result = reviewgraph_export.Result

type Workflow struct {
	service reviewgraph_export.Service
}

func New(service reviewgraph_export.Service) Workflow {
	return Workflow{service: service}
}

func (w Workflow) Run(req Request) (Result, error) {
	return w.service.Export(reviewgraph_export.Request(req))
}
