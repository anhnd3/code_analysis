package build_snapshot

import (
	"strconv"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	cacheport "analysis-module/internal/ports/cache"
	graphstoreport "analysis-module/internal/ports/graphstore"
	"analysis-module/internal/services/graph_build"
	"analysis-module/internal/services/quality_report"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/internal/workflows/analyze_workspace"
)

type Request struct {
	WorkspacePath  string   `json:"workspace_path"`
	IgnorePatterns []string `json:"ignore_patterns"`
	TargetHints    []string `json:"target_hints"`
}

type Result struct {
	WorkspaceID   string                        `json:"workspace_id"`
	Snapshot      graph.GraphSnapshot           `json:"snapshot"`
	QualityReport quality.AnalysisQualityReport `json:"quality_report"`
	ArtifactRefs  []artifact.Ref                `json:"artifact_refs"`
}

type Workflow struct {
	analyze        analyze_workspace.Workflow
	symbolIndex    symbol_index.Service
	graphBuild     graph_build.Service
	graphStores    graphstoreport.Provider
	cache          cacheport.SnapshotCache
	artifactStore  artifactstoreport.Store
	qualityReport  quality_report.Service
	snapshotManage snapshot_manage.Service
	reporter       progress.Reporter
}

func New(analyze analyze_workspace.Workflow, symbolIndex symbol_index.Service, graphBuild graph_build.Service, graphStores graphstoreport.Provider, cache cacheport.SnapshotCache, artifactStore artifactstoreport.Store, qualityReport quality_report.Service, snapshotManage snapshot_manage.Service, reporter progress.Reporter) Workflow {
	if reporter == nil {
		reporter = progress.NoopReporter{}
	}
	return Workflow{
		analyze:        analyze,
		symbolIndex:    symbolIndex,
		graphBuild:     graphBuild,
		graphStores:    graphStores,
		cache:          cache,
		artifactStore:  artifactStore,
		qualityReport:  qualityReport,
		snapshotManage: snapshotManage,
		reporter:       reporter,
	}
}

func (w Workflow) Run(req Request) (Result, error) {
	analyzeResult, err := w.analyze.Run(analyze_workspace.Request{
		WorkspacePath:  req.WorkspacePath,
		IgnorePatterns: req.IgnorePatterns,
		TargetHints:    req.TargetHints,
	})
	if err != nil {
		return Result{}, err
	}
	snapshotID := w.snapshotManage.NewSnapshotID(string(analyzeResult.WorkspaceManifest.ID))
	extraction, err := w.symbolIndex.Build(analyzeResult.Inventory)
	if err != nil {
		return Result{}, err
	}
	graphResult := w.graphBuild.Build(string(analyzeResult.WorkspaceManifest.ID), snapshotID, analyzeResult.Inventory, extraction)
	store, err := w.graphStores.ForWorkspace(string(analyzeResult.WorkspaceManifest.ID))
	if err != nil {
		return Result{}, err
	}
	w.reporter.StartStage("persist", 0)
	if err := store.SaveSnapshot(graphResult.Snapshot); err != nil {
		w.reporter.FinishStage("persist failed")
		return Result{}, err
	}
	w.cache.Put(graphResult.Snapshot)
	artifactRefs := append([]artifact.Ref{}, analyzeResult.ArtifactRefs...)
	graphRefs, err := w.artifactStore.SaveGraph(string(analyzeResult.WorkspaceManifest.ID), snapshotID, graphResult.Snapshot.Nodes, graphResult.Snapshot.Edges)
	if err == nil {
		artifactRefs = append(artifactRefs, graphRefs...)
	}
	report := w.qualityReport.Build(snapshotID, analyzeResult.Inventory, extraction, graphResult)
	if ref, err := w.artifactStore.SaveQualityReport(string(analyzeResult.WorkspaceManifest.ID), snapshotID, report); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	w.reporter.FinishStage("artifacts=" + strconv.Itoa(len(artifactRefs)))
	return Result{
		WorkspaceID:   string(analyzeResult.WorkspaceManifest.ID),
		Snapshot:      graphResult.Snapshot,
		QualityReport: report,
		ArtifactRefs:  artifactRefs,
	}, nil
}
