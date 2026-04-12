package review_graph_list_startpoints

import "analysis-module/internal/services/reviewgraph_select"

type Request struct {
	DBPath  string `json:"db_path"`
	Mode    string `json:"mode"`
	Symbol  string `json:"symbol,omitempty"`
	File    string `json:"file,omitempty"`
	Topic   string `json:"topic,omitempty"`
	OutPath string `json:"out_path,omitempty"`
}

type Result = reviewgraph_select.Result

type Workflow struct {
	service reviewgraph_select.Service
}

func New(service reviewgraph_select.Service) Workflow {
	return Workflow{service: service}
}

func (w Workflow) Run(req Request) (Result, error) {
	return w.service.Select(reviewgraph_select.Request(req))
}
