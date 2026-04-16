package bootstrap

import (
	"log/slog"
	"net/http"

	httpapi "analysis-module/internal/adapters/api/http"
	artifactfs "analysis-module/internal/adapters/artifactstore/filesystem"
	goextractor "analysis-module/internal/adapters/extractor/go"
	jsextractor "analysis-module/internal/adapters/extractor/javascript"
	pythonextractor "analysis-module/internal/adapters/extractor/python"
	cachememory "analysis-module/internal/adapters/graphstore/memory"
	sqliteprovider "analysis-module/internal/adapters/graphstore/sqlite"
	scannerdetectors "analysis-module/internal/adapters/scanner/detectors"
	"analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/boundary/go/frameworks"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/progress"
	"analysis-module/internal/services/graph_build"
	"analysis-module/internal/services/graph_query"
	"analysis-module/internal/services/quality_report"
	"analysis-module/internal/services/repo_inventory"
	"analysis-module/internal/services/reviewgraph_export"
	"analysis-module/internal/services/reviewgraph_import"
	"analysis-module/internal/services/reviewgraph_paths"
	"analysis-module/internal/services/reviewgraph_select"
	"analysis-module/internal/services/reviewgraph_traverse"
	"analysis-module/internal/services/review_bundle_packager"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/internal/services/workspace_scan"
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_review_bundle"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
	"analysis-module/internal/workflows/review_graph_export"
	"analysis-module/internal/workflows/review_graph_import"
	"analysis-module/internal/workflows/review_graph_list_startpoints"
	"analysis-module/internal/workflows/export_mermaid"
	"analysis-module/internal/services/entrypoint_resolve"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/cross_boundary_link"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/sequence_model_build"
	"analysis-module/internal/services/mermaid_emit"
	"analysis-module/internal/services/boundary_detect"
)

type Application struct {
	Config            config.Config
	Logger            *slog.Logger
	AnalyzeWorkspace  analyze_workspace.Workflow
	BuildSnapshot     build_snapshot.Workflow
	BuildReviewBundle build_review_bundle.Workflow
	BlastRadius       blast_radius.Workflow
	ImpactedTests     impacted_tests.Workflow
	ReviewGraphImport review_graph_import.Workflow
	ReviewGraphListStartpoints review_graph_list_startpoints.Workflow
	ReviewGraphExport review_graph_export.Workflow
	ExportMermaid     export_mermaid.Workflow
	HTTPHandler       http.Handler
}

func New(cfg config.Config, logger *slog.Logger) (*Application, error) {
	artifactStore := artifactfs.New(cfg.ArtifactRoot)
	reporter := progress.NewStderrReporter(cfg.ProgressMode)
	graphStores := sqliteprovider.NewProvider(cfg.ArtifactRoot, cfg.SQLitePath)
	cache := cachememory.NewSnapshotCache()
	snapshotManageSvc := snapshot_manage.New()
	workspaceScanSvc := workspace_scan.New(
		scannerdetectors.NewRepoRootDetector(reporter),
		scannerdetectors.NewTechStackDetector(),
		scannerdetectors.NewServiceDetector(),
		reporter,
	)
	inventorySvc := repo_inventory.New()
	analyzeWorkflow := analyze_workspace.New(workspaceScanSvc, inventorySvc, artifactStore, snapshotManageSvc)
	symbolIndexSvc := symbol_index.New(reporter, goextractor.New(), pythonextractor.New(), jsextractor.New())
	graphBuildSvc := graph_build.New(reporter)
	
	boundaryRegistry := boundary.NewRegistry()
	boundaryRegistry.Register(frameworks.NewGinDetector())
	boundaryRegistry.Register(frameworks.NewNetHTTPDetector())
	boundaryRegistry.Register(frameworks.NewGRPCGatewayDetector())
	boundaryDetectSvc := boundary_detect.New(boundaryRegistry)

	qualityReportSvc := quality_report.New()
	buildSnapshotWorkflow := build_snapshot.New(analyzeWorkflow, symbolIndexSvc, graphBuildSvc, graphStores, cache, artifactStore, qualityReportSvc, snapshotManageSvc, boundaryDetectSvc, reporter)
	reviewBundlePackager := review_bundle_packager.New(cfg.ArtifactRoot, reporter)
	buildReviewBundleWorkflow := build_review_bundle.New(buildSnapshotWorkflow, reviewBundlePackager)
	querySvc := graph_query.New(graphStores)
	blastRadiusWorkflow := blast_radius.New(querySvc, artifactStore)
	impactedTestsWorkflow := impacted_tests.New(querySvc, artifactStore)
	reviewGraphPathsSvc := reviewgraph_paths.New(cfg.ArtifactRoot)
	reviewGraphImportSvc := reviewgraph_import.New(reviewGraphPathsSvc)
	reviewGraphTraverseSvc := reviewgraph_traverse.New()
	reviewGraphSelectSvc := reviewgraph_select.New(reviewGraphPathsSvc)
	reviewGraphExportSvc := reviewgraph_export.New(reviewGraphPathsSvc, reviewGraphTraverseSvc)
	
	entrypointResolveSvc := entrypoint_resolve.New()
	flowStitchSvc := flow_stitch.New()
	crossBoundaryLinkSvc := cross_boundary_link.New()
	chainReduceSvc := chain_reduce.New()
	sequenceModelSvc := sequence_model_build.New()
	mermaidEmitSvc := mermaid_emit.New()
	exportMermaidWorkflow := export_mermaid.New(graphStores, artifactStore, entrypointResolveSvc, flowStitchSvc, crossBoundaryLinkSvc, chainReduceSvc, sequenceModelSvc, mermaidEmitSvc, boundaryDetectSvc)

	return &Application{
		Config:            cfg,
		Logger:            logger,
		AnalyzeWorkspace:  analyzeWorkflow,
		BuildSnapshot:     buildSnapshotWorkflow,
		BuildReviewBundle: buildReviewBundleWorkflow,
		BlastRadius:       blastRadiusWorkflow,
		ImpactedTests:     impactedTestsWorkflow,
		ReviewGraphImport: review_graph_import.New(reviewGraphImportSvc),
		ReviewGraphListStartpoints: review_graph_list_startpoints.New(reviewGraphSelectSvc),
		ReviewGraphExport: review_graph_export.New(reviewGraphExportSvc),
		ExportMermaid:     exportMermaidWorkflow,
		HTTPHandler:       httpapi.New(analyzeWorkflow, buildSnapshotWorkflow, blastRadiusWorkflow, impactedTestsWorkflow),
	}, nil
}
