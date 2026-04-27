package bootstrap

import (
	"log/slog"	
	"time"

	artifactfs "analysis-module/internal/adapters/artifactstore/filesystem"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/progress"
	factmarkdown "analysis-module/internal/export/markdown"
	factmermaid "analysis-module/internal/export/mermaid"
	"analysis-module/internal/llm"
	factquery "analysis-module/internal/query"
	factreview "analysis-module/internal/review"
	"analysis-module/internal/services/boundary_detect"
	"analysis-module/internal/services/snapshot_manage"
	"analysis-module/internal/services/workspace_scan"
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/facts_index"
)

type Application struct {
	Config       config.Config
	Logger       *slog.Logger

	Scan         analyze_workspace.Workflow // or renamed WorkspaceScan later
	FactsIndex   facts_index.Workflow
	FactsQuery   factquery.Service
	FlowReview   factreview.Service
	FlowMarkdown factmarkdown.Service
	FlowMermaid  factmermaid.Service
}

func New(cfg config.Config, logger *slog.Logger) (*Application, error) {
	artifactStore := artifactfs.New(cfg.ArtifactRoot)
	reporter := progress.NewStderrReporter(cfg.ProgressMode)
	snapshotManageSvc := snapshot_manage.New()
	workspaceScanSvc := workspace_scan.New(
		reporter,
	)
	analyzeWorkflow := analyze_workspace.New(workspaceScanSvc, artifactStore, snapshotManageSvc)

	boundaryRegistry := boundary.NewRegistry()
	boundaryRegistry.Register(frameworks.NewGinDetector())
	boundaryRegistry.Register(frameworks.NewNetHTTPDetector())
	boundaryRegistry.Register(frameworks.NewGRPCGatewayDetector())
	boundaryDetectSvc := boundary_detect.New(boundaryRegistry)

	factsIndexWorkflow := facts_index.New(analyzeWorkflow, boundaryDetectSvc, snapshotManageSvc, artifactStore, cfg.ArtifactRoot)
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
		FlowMarkdown: factMarkdownSvc,
		FlowMermaid:  flowMermaidSvc,
	}, nil
}