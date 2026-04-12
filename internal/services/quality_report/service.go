package quality_report

import (
	"analysis-module/internal/domain/analysis"
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
	unsupportedLangs := 0
	for _, repo := range inventory.Repositories {
		for _, lang := range repo.TechStack.Languages {
			if lang != repository.LanguageGo && lang != repository.LanguagePython && lang != repository.LanguageJS && lang != repository.LanguageTS {
				unsupportedLangs++
			}
		}
	}
	issueCounts := graphResult.IssueCounts
	if issueCounts.DeferredBoundaryStitching > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "boundary_stitching",
			Message:  "boundary hints detected but cross-service stitching is deferred in this pass",
			Count:    issueCounts.DeferredBoundaryStitching,
		})
	}
	if unsupportedLangs > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "language_support",
			Message:  "repositories using unsupported languages were discovered and only supported languages were extracted",
			Count:    unsupportedLangs,
		})
	}
	if issueCounts.UnresolvedImports > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "call_resolution",
			Message:  "some call sites or imports were not resolved into local graph edges",
			Count:    issueCounts.UnresolvedImports,
		})
	}
	if issueCounts.AmbiguousRelations > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "ambiguous_relations",
			Message:  "some statically visible call targets remained ambiguous and were not forced into graph edges",
			Count:    issueCounts.AmbiguousRelations,
		})
	}
	if issueCounts.UnsupportedConstructs > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "unsupported_constructs",
			Message:  "some language constructs exceeded the supported static analysis ceiling",
			Count:    issueCounts.UnsupportedConstructs,
		})
	}
	if issueCounts.ServiceAttributionAmbiguities > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "service_attribution",
			Message:  "some files or symbols remained unassigned because service ownership was ambiguous or shared",
			Count:    issueCounts.ServiceAttributionAmbiguities,
		})
	}
	if issueCounts.SkippedIgnoredFiles > 0 {
		gaps = append(gaps, quality.GapReport{
			Category: "ignored_content",
			Message:  "some files were skipped because of the active ignore policy",
			Count:    issueCounts.SkippedIgnoredFiles,
		})
	}
	return quality.AnalysisQualityReport{
		SnapshotID: snapshotID,
		Metrics: []quality.CoverageMetric{
			{Name: "repositories", Value: len(inventory.Repositories), Total: len(inventory.Repositories)},
			{Name: "source_files_parsed", Value: parsedFiles, Total: sourceFiles},
			{Name: "symbols_indexed", Value: graphResult.Snapshot.Metadata.SymbolCount, Total: graphResult.Snapshot.Metadata.SymbolCount},
		},
		IssueCounts: analysis.IssueCounts{
			UnresolvedImports:             issueCounts.UnresolvedImports,
			AmbiguousRelations:            issueCounts.AmbiguousRelations,
			UnsupportedConstructs:         issueCounts.UnsupportedConstructs,
			SkippedIgnoredFiles:           issueCounts.SkippedIgnoredFiles,
			DeferredBoundaryStitching:     issueCounts.DeferredBoundaryStitching,
			ServiceAttributionAmbiguities: issueCounts.ServiceAttributionAmbiguities,
		},
		Gaps: gaps,
	}
}
