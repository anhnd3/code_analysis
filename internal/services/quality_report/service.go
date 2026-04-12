package quality_report

import (
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/services/graph_build"
	"analysis-module/internal/services/symbol_index"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Build(snapshotID string, inventory repository.Inventory, extraction symbol_index.Result, graphResult graph_build.BuildResult) quality.AnalysisQualityReport {
	sourceFiles := 0
	for _, repo := range inventory.Repositories {
		sourceFiles += len(repo.GoFiles) + len(repo.PythonFiles) + len(repo.JavaScriptFiles) + len(repo.TypeScriptFiles)
	}
	parsedFiles := 0
	for _, repo := range extraction.Repositories {
		parsedFiles += len(repo.Files)
	}
	gaps := []quality.GapReport{}
	deferredBoundaryCount := 0
	unsupportedLangs := 0
	for _, repo := range inventory.Repositories {
		if len(repo.BoundaryHints) > 0 {
			deferredBoundaryCount += len(repo.BoundaryHints)
		}
		for _, lang := range repo.TechStack.Languages {
			if lang != repository.LanguageGo && lang != repository.LanguagePython && lang != repository.LanguageJS && lang != repository.LanguageTS {
				unsupportedLangs++
			}
		}
	}
	if deferredBoundaryCount > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "boundary_stitching",
			Message:  "boundary hints detected but cross-service stitching is deferred in this pass",
			Count:    deferredBoundaryCount,
		})
	}
	if unsupportedLangs > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "language_support",
			Message:  "repositories using unsupported languages were discovered and only supported languages were extracted",
			Count:    unsupportedLangs,
		})
	}
	if graphResult.UnresolvedCalls > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "call_resolution",
			Message:  "some call sites were not resolved into local graph edges",
			Count:    graphResult.UnresolvedCalls,
		})
	}
	return quality.AnalysisQualityReport{
		SnapshotID: snapshotID,
		Metrics: []quality.CoverageMetric{
			{Name: "repositories", Value: len(inventory.Repositories), Total: len(inventory.Repositories)},
			{Name: "source_files_parsed", Value: parsedFiles, Total: sourceFiles},
			{Name: "symbols_indexed", Value: graphResult.Snapshot.Metadata.SymbolCount, Total: graphResult.Snapshot.Metadata.SymbolCount},
		},
		Gaps: gaps,
	}
}
