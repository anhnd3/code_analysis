package impacted_tests

import (
	"analysis-module/internal/domain/artifact"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	queryport "analysis-module/internal/ports/query"
)

type Request struct {
	WorkspaceID string `json:"workspace_id"`
	SnapshotID  string `json:"snapshot_id"`
	Target      string `json:"target"`
	MaxDepth    int    `json:"max_depth"`
}

type Result struct {
	QueryResult  queryport.ImpactedTestsResult `json:"query_result"`
	ArtifactRefs []artifact.Ref                `json:"artifact_refs"`
}

type Workflow struct {
	queryService  queryport.Service
	artifactStore artifactstoreport.Store
}

func New(queryService queryport.Service, artifactStore artifactstoreport.Store) Workflow {
	return Workflow{queryService: queryService, artifactStore: artifactStore}
}

func (w Workflow) Run(req Request) (Result, error) {
	result, err := w.queryService.ImpactedTests(queryport.ImpactedTestsRequest{
		WorkspaceID: req.WorkspaceID,
		SnapshotID:  req.SnapshotID,
		Target:      req.Target,
		MaxDepth:    req.MaxDepth,
	})
	if err != nil {
		return Result{}, err
	}
	refs := []artifact.Ref{}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "impacted_tests.json", artifact.TypeImpactedTests, result); err == nil {
		refs = append(refs, ref)
	}
	return Result{QueryResult: result, ArtifactRefs: refs}, nil
}
