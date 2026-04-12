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
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/progress"
	"analysis-module/internal/services/graph_build"
	"analysis-module/internal/services/graph_query"
	"analysis-module/internal/services/quality_report"
	"analysis-module/internal/services/repo_inventory"
	"analysis-module/internal/services/review_bundle_packager"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/internal/services/workspace_scan"
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_review_bundle"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
)

type Application struct {
	Config            config.Config
	Logger            *slog.Logger
	AnalyzeWorkspace  analyze_workspace.Workflow
	BuildSnapshot     build_snapshot.Workflow
	BuildReviewBundle build_review_bundle.Workflow
	BlastRadius       blast_radius.Workflow
	ImpactedTests     impacted_tests.Workflow
	HTTPHandler       http.Handler
}

func New(cfg config.Config, logger *slog.Logger) (*Application, error) {
	artifactStore := artifactfs.New(cfg.ArtifactRoot)
	reporter := progress.NewStderrReporter()
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
	qualityReportSvc := quality_report.New()
	buildSnapshotWorkflow := build_snapshot.New(analyzeWorkflow, symbolIndexSvc, graphBuildSvc, graphStores, cache, artifactStore, qualityReportSvc, snapshotManageSvc, reporter)
	reviewBundlePackager := review_bundle_packager.New(cfg.ArtifactRoot, reporter)
	buildReviewBundleWorkflow := build_review_bundle.New(buildSnapshotWorkflow, reviewBundlePackager)
	querySvc := graph_query.New(graphStores)
	blastRadiusWorkflow := blast_radius.New(querySvc, artifactStore)
	impactedTestsWorkflow := impacted_tests.New(querySvc, artifactStore)
	return &Application{
		Config:            cfg,
		Logger:            logger,
		AnalyzeWorkspace:  analyzeWorkflow,
		BuildSnapshot:     buildSnapshotWorkflow,
		BuildReviewBundle: buildReviewBundleWorkflow,
		BlastRadius:       blastRadiusWorkflow,
		ImpactedTests:     impactedTestsWorkflow,
		HTTPHandler:       httpapi.New(analyzeWorkflow, buildSnapshotWorkflow, blastRadiusWorkflow, impactedTestsWorkflow),
	}, nil
}
