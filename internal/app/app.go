package app

import (
	"log/slog"
	"time"

	artifactfs "analysis-module/internal/adapters/artifactstore/filesystem"
	"analysis-module/internal/export"
	factquery "analysis-module/internal/facts/query"
	"analysis-module/internal/indexer"
	"analysis-module/internal/llm"
	factreview "analysis-module/internal/review"
	"analysis-module/internal/services/snapshot_manage"
)

// Application holds the wired-up services and workflows.
type Application struct {
	Config Config
	Logger *slog.Logger

	Scan          indexer.ScanWorkflow
	FactsIndex    indexer.IndexWorkflow
	FactsQuery    factquery.Service
	FlowReview    factreview.Service
	FlowMarkdown  export.MarkdownService
	FlowMermaid   export.MermaidService
	FlowGraphJSON export.GraphJSONService
}

// New constructs a fully wired Application.
func New(cfg Config, logger *slog.Logger) (*Application, error) {
	artifactStore := artifactfs.New(cfg.ArtifactRoot)
	reporter := NewProgressReporter(cfg.ProgressMode)
	snapshotManageSvc := snapshot_manage.New()

	workflows, err := indexer.NewDefaultWorkflows(indexer.WorkflowOptions{
		ArtifactRoot:   cfg.ArtifactRoot,
		ArtifactStore:  artifactStore,
		SnapshotManage: snapshotManageSvc,
		Reporter:       reporter,
	})
	if err != nil {
		return nil, err
	}

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
	flowMarkdownSvc := export.NewMarkdownService()
	flowMermaidSvc := export.NewMermaidService()
	flowGraphJSONSvc := export.NewGraphJSONService()

	return &Application{
		Config:        cfg,
		Logger:        logger,
		Scan:          workflows.Scan,
		FactsIndex:    workflows.Index,
		FactsQuery:    factsQuerySvc,
		FlowReview:    flowReviewSvc,
		FlowMarkdown:  flowMarkdownSvc,
		FlowMermaid:   flowMermaidSvc,
		FlowGraphJSON: flowGraphJSONSvc,
	}, nil
}
