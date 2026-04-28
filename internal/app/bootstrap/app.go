package bootstrap

import (
	"log/slog"
	"time"

	artifactfs "analysis-module/internal/adapters/artifactstore/filesystem"
	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/boundary/go/frameworks"
	goextractor "analysis-module/internal/adapters/extractor/go"
	jsextractor "analysis-module/internal/adapters/extractor/javascript"
	pythonextractor "analysis-module/internal/adapters/extractor/python"
	"analysis-module/internal/adapters/scanner/detectors"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/progress"
	factmarkdown "analysis-module/internal/export/markdown"
	factmermaid "analysis-module/internal/export/mermaid"
	"analysis-module/internal/indexer/extract/boundaries"
	"analysis-module/internal/indexer/extract/symbols"
	"analysis-module/internal/indexer/scan/inventory"
	"analysis-module/internal/indexer/scan/workspace"
	"analysis-module/internal/indexer/workflow/index"
	"analysis-module/internal/indexer/workflow/scan"
	"analysis-module/internal/llm"
	factquery "analysis-module/internal/query"
	factreview "analysis-module/internal/review"
	"analysis-module/internal/services/snapshot_manage"
)

import (
	"log/slog"
	"time"

	artifactfs "analysis-module/internal/adapters/artifactstore/filesystem"
	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/boundary/go/frameworks"
	goextractor "analysis-module/internal/adapters/extractor/go"
	jsextractor "analysis-module/internal/adapters/extractor/javascript"
	pythonextractor "analysis-module/internal/adapters/extractor/python"
	"analysis-module/internal/adapters/scanner/detectors"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/progress"
	factmarkdown "analysis-module/internal/export/markdown"
	factmermaid "analysis-module/internal/export/mermaid"
	"analysis-module/internal/indexer/extract/boundaries"
	"analysis-module/internal/indexer/extract/symbols"
	"analysis-module/internal/indexer/scan/inventory"
	"analysis-module/internal/indexer/scan/workspace"
	"analysis-module/internal/indexer/workflow/index"
	"analysis-module/internal/indexer/workflow/scan"
	"analysis-module/internal/llm"
	factquery "analysis-module/internal/query"
	factreview "analysis-module/internal/review"
	"analysis-module/internal/services/snapshot_manage"

	boundaryidx "analysis-module/internal/indexer/extract/boundaries"
	symbolidx "analysis-module/internal/indexer/extract/symbols"
	inventoryidx "analysis-module/internal/indexer/scan/inventory"
	workspaceidx "analysis-module/internal/indexer/scan/workspace"
	indexworkflow "analysis-module/internal/indexer/workflow/index"
	scanworkflow "analysis-module/internal/indexer/workflow/scan"
)

type Application struct {
	Config config.Config
	Logger *slog.Logger

	Scan         scanworkflow.Workflow // or renamed WorkspaceScan later
	FactsIndex   indexworkflow.Workflow
	FactsQuery   factquery.Service
	FlowReview   factreview.Service
	FlowMarkdown factmarkdown.Service
	FlowMermaid  factmermaid.Service
}

func New(cfg config.Config, logger *slog.Logger) (*Application, error) {
	artifactStore := artifactfs.New(cfg.ArtifactRoot)
	reporter := progress.NewStderrReporter(cfg.ProgressMode)
	snapshotManageSvc := snapshot_manage.New()
	workspaceScanSvc := workspaceidx.New(
		detectors.NewRepoRootDetector(reporter),
		detectors.NewTechStackDetector(),
		detectors.NewServiceDetector(),
		reporter,
	)
	inventory := inventoryidx.New()
	analyzeWorkflow := scanworkflow.New(workspaceScanSvc, inventory, artifactStore, snapshotManageSvc)

	boundaryRegistry := boundary.NewRegistry()
	boundaryRegistry.Register(frameworks.NewGinDetector())
	boundaryRegistry.Register(frameworks.NewNetHTTPDetector())
	boundaryRegistry.Register(frameworks.NewGRPCGatewayDetector())
	boundaryDetectSvc := boundaryidx.New(boundaryRegistry)

	symbolIdx := symbolidx.New(
		reporter,
		goextractor.New(),
		pythonextractor.New(),
		jsextractor.New(),
	)
	factsIndexWorkflow := indexworkflow.New(analyzeWorkflow, symbolIdx, boundaryDetectSvc, snapshotManageSvc, artifactStore, cfg.ArtifactRoot)
	factsQuerySvc := factquery.New(cfg.ArtifactRoot)
	var llmClient llm.Client = llm.NoopClient{}
	if cfg.LLMBaseURL != "" && cfg.LLMModel != "" {
		llmClient = llm.OpenAIClient{
			BaseURL:    cfg.LLMBaseURL,
			Model:      cfg.LLMModel,
			APIKey:     cfg.LLMAPIKey,
			Timeout:    time.Duration(cfg.LLMTimeoutSec) * time.Second,
			MaxRetries: cfg.LLMMaxRetries,
		}
	}
	flowReviewSvc := factreview.New(cfg.ArtifactRoot, factsQuerySvc, llmClient)
	flowMarkdownSvc := factmarkdown.New()
	flowMermaidSvc := factmermaid.New()

	return &Application{
		Config:       cfg,
		Logger:       logger,
		Scan:         analyzeWorkflow,
		FactsIndex:   factsIndexWorkflow,
		FactsQuery:   factsQuerySvc,
		FlowReview:   flowReviewSvc,
		FlowMarkdown: flowMarkdownSvc,
		FlowMermaid:  flowMermaidSvc,
	}, nil
}
