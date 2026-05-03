package indexer

import (
	"fmt"
	"path/filepath"
	"strconv"

	"analysis-module/internal/facts"
)

// ToRepositoryFacts converts inventory manifests to repository facts.
func ToRepositoryFacts(inventory Inventory) []facts.RepositoryFact {
	if len(inventory.Repositories) == 0 {
		return nil
	}
	result := make([]facts.RepositoryFact, 0, len(inventory.Repositories))
	for _, repo := range inventory.Repositories {
		f := facts.RepositoryFact{
			ID:       string(repo.ID),
			Name:     repo.Name,
			RootPath: repo.RootPath,
			Role:     string(repo.Role),
		}
		for _, lang := range repo.TechStack.Languages {
			f.Languages = append(f.Languages, string(lang))
		}
		f.BuildFiles = append(f.BuildFiles, repo.TechStack.BuildFiles...)
		if len(repo.TechStack.FrameworkHints) > 0 {
			f.Frameworks = append(f.Frameworks, repo.TechStack.FrameworkHints...)
		}
		result = append(result, f)
	}
	return result
}

// ToServiceFacts converts candidate service manifests to service facts.
func ToServiceFacts(inventory Inventory) []facts.ServiceFact {
	var result []facts.ServiceFact
	for _, repo := range inventory.Repositories {
		for _, svc := range repo.CandidateServices {
			f := facts.ServiceFact{
				ID:           string(svc.ID),
				RepositoryID: string(repo.ID),
				Name:         svc.Name,
				RootPath:     svc.RootPath,
				Entrypoints:  append([]string{}, svc.Entrypoints...),
			}
			for _, b := range svc.Boundaries {
				f.BoundaryHints = append(f.BoundaryHints, string(b))
			}
			result = append(result, f)
		}
	}
	return result
}

// ToFileFacts converts extraction file results to file facts.
func ToFileFacts(extraction ExtractionResult) []facts.FileFact {
	var result []facts.FileFact
	for _, repoExt := range extraction.Repositories {
		repoRoot := filepath.ToSlash(repoExt.Repository.RootPath)
		repoID := string(repoExt.Repository.ID)
		for fi, fileExt := range repoExt.Files {
			f := facts.FileFact{
				ID:             Stable("file", repoRoot, fileExt.FilePath),
				RepositoryID:   repoID,
				RepositoryRoot: repoRoot,
				RelativePath:   filepath.ToSlash(fileExt.FilePath),
				AbsolutePath:   filepath.Join(repoRoot, fileExt.FilePath),
				Language:       string(LanguageGo), // default; may be overridden by extractor
			}
			if fileExt.Language != "" {
				f.Language = fileExt.Language
			} else {
				// Derive language from file extension when not set by extractor
				ext := filepath.Ext(fileExt.FilePath)
				switch ext {
				case ".py":
					f.Language = "python"
				case ".js":
					f.Language = "javascript"
				case ".ts":
					f.Language = "typescript"
				}
			}
			f.PackageName = fileExt.PackageName
			result = append(result, f)
			_ = fi // used for debugging if needed
		}
	}
	return result
}

// ToSymbolFacts converts extraction symbols to symbol facts.
func ToSymbolFacts(extraction ExtractionResult) []facts.SymbolFact {
	var result []facts.SymbolFact
	for _, repoExt := range extraction.Repositories {
		repoID := string(repoExt.Repository.ID)
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			for _, sym := range fileExt.Symbols {
				f := facts.SymbolFact{
					ID:            string(sym.ID),
					RepositoryID:  repoID,
					FileID:        Stable("file", repoExt.Repository.RootPath, filePath),
					FilePath:      filePath,
					CanonicalName: sym.CanonicalName,
					Name:          sym.Name,
					Receiver:      sym.Receiver,
					Kind:          string(sym.Kind),
					Signature:     sym.Signature,
					StartLine:     sym.Location.StartLine,
					StartCol:      sym.Location.StartCol,
					EndLine:       sym.Location.EndLine,
					EndCol:        sym.Location.EndCol,
					StartByte:     sym.Location.StartByte,
					EndByte:       sym.Location.EndByte,
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToImportFacts converts extraction import bindings to import facts.
func ToImportFacts(extraction ExtractionResult) []facts.ImportFact {
	var result []facts.ImportFact
	for _, repoExt := range extraction.Repositories {
		repoID := string(repoExt.Repository.ID)
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			fileID := Stable("file", repoExt.Repository.RootPath, filePath)
			offset := 0
			for i, imp := range fileExt.ImportBindings {
				f := facts.ImportFact{
					ID:           Stable("import", repoID, filePath, strconv.Itoa(i)),
					FileID:       fileID,
					FilePath:     filePath,
					Source:       imp.Source,
					Alias:        imp.Alias,
					ImportedName: imp.ImportedName,
					ExportName:   imp.ExportName,
					ResolvedPath: imp.ResolvedPath,
					IsDefault:    imp.IsDefault,
					IsNamespace:  imp.IsNamespace,
					IsLocal:      imp.IsLocal,
				}
				result = append(result, f)
				offset++
			}
			for i, imp := range fileExt.Imports {
				if imp == "" {
					continue
				}
				f := facts.ImportFact{
					ID:       Stable("import", repoID, filePath, "raw", strconv.Itoa(offset+i)),
					FileID:   fileID,
					FilePath: filePath,
					Source:   imp,
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToExportFacts converts extraction exports to export facts.
func ToExportFacts(extraction ExtractionResult) []facts.ExportFact {
	var result []facts.ExportFact
	for _, repoExt := range extraction.Repositories {
		repoID := string(repoExt.Repository.ID)
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			fileID := Stable("file", repoExt.Repository.RootPath, filePath)
			for i, exp := range fileExt.Exports {
				f := facts.ExportFact{
					ID:            Stable("export", repoID, filePath, strconv.Itoa(i)),
					FileID:        fileID,
					FilePath:      filePath,
					Name:          exp.Name,
					CanonicalName: exp.CanonicalName,
					IsDefault:     exp.IsDefault,
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToCallCandidates converts relation candidates to call candidate facts.
func ToCallCandidates(extraction ExtractionResult) []facts.CallCandidate {
	var result []facts.CallCandidate
	for _, repoExt := range extraction.Repositories {
		repoID := string(repoExt.Repository.ID)
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			for i, rel := range fileExt.Relations {
				f := facts.CallCandidate{
					ID:                  Stable("call", repoID, filePath, strconv.Itoa(i)),
					SourceSymbolID:      string(rel.SourceSymbolID),
					TargetCanonicalName: rel.TargetCanonicalName,
					TargetFilePath:      filepath.ToSlash(rel.TargetFilePath),
					TargetExportName:    rel.TargetExportName,
					Relationship:        rel.Relationship,
					EvidenceType:        rel.EvidenceType,
					EvidenceSource:      rel.EvidenceSource,
					ExtractionMethod:    rel.ExtractionMethod,
					ConfidenceScore:     rel.ConfidenceScore,
					OrderIndex:          rel.OrderIndex,
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToExecutionHints converts extraction hints to execution hint facts.
func ToExecutionHints(extraction ExtractionResult) []facts.ExecutionHintFact {
	var result []facts.ExecutionHintFact
	for _, repoExt := range extraction.Repositories {
		repoID := string(repoExt.Repository.ID)
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			for i, hint := range fileExt.Hints {
				f := facts.ExecutionHintFact{
					ID:             Stable("hint", repoID, filePath, strconv.Itoa(i)),
					FilePath:       filePath,
					SourceSymbolID: hint.SourceSymbolID,
					TargetSymbolID: hint.TargetSymbolID,
					TargetSymbol:   hint.TargetSymbol,
					Kind:           string(hint.Kind),
					Evidence:       hint.Evidence,
					OrderIndex:     hint.OrderIndex, // Preserve original order index from extractor
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToDiagnosticFacts converts extraction diagnostics to diagnostic facts.
func ToDiagnosticFacts(extraction ExtractionResult) []facts.DiagnosticFact {
	var result []facts.DiagnosticFact
	for _, repoExt := range extraction.Repositories {
		repoID := string(repoExt.Repository.ID)
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			for i, diag := range fileExt.Diagnostics {
				f := facts.DiagnosticFact{
					ID:       Stable("diag", repoID, filePath, strconv.Itoa(i)),
					FilePath: filePath,
					SymbolID: string(diag.SymbolID),
					Category: diag.Category,
					Message:  diag.Message,
					Evidence: diag.Evidence,
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToBoundaryFacts converts boundary detection roots to boundary facts.
func ToBoundaryFacts(boundaryResult BoundaryDetectionResult) []facts.BoundaryFact {
	if len(boundaryResult.Roots) == 0 {
		return nil
	}
	result := make([]facts.BoundaryFact, 0, len(boundaryResult.Roots))
	for _, root := range boundaryResult.Roots {
		f := facts.BoundaryFact{
			ID:              root.ID,
			RepositoryID:    root.RepositoryID,
			Kind:            string(root.Kind),
			Framework:       root.Framework,
			Method:          root.Method,
			Path:            root.Path,
			CanonicalName:   root.CanonicalName,
			SourceFile:      root.SourceFile,
			SourceExpr:      root.SourceExpr,
			HandlerTarget:   root.HandlerTarget,
			SourceStartByte: root.SourceStartByte,
			SourceEndByte:   root.SourceEndByte,
		}
		result = append(result, f)
	}
	return result
}

// ToBoundaryDiagnostics converts boundary detection diagnostics to diagnostic facts.
func ToBoundaryDiagnostics(boundaryResult BoundaryDetectionResult) []facts.DiagnosticFact {
	if len(boundaryResult.Diagnostics) == 0 {
		return nil
	}
	result := make([]facts.DiagnosticFact, 0, len(boundaryResult.Diagnostics))
	for i, diag := range boundaryResult.Diagnostics {
		f := facts.DiagnosticFact{
			ID:       Stable("bd", "diag", strconv.Itoa(i)),
			FilePath: diag.FilePath,
			SymbolID: string(diag.SymbolID),
			Category: diag.Category,
			Message:  diag.Message,
			Evidence: diag.Evidence,
		}
		result = append(result, f)
	}
	return result
}

// ToTestFacts extracts test functions from extraction results as test facts.
func ToTestFacts(extraction ExtractionResult) []facts.TestFact {
	var result []facts.TestFact
	for _, repoExt := range extraction.Repositories {
		for _, fileExt := range repoExt.Files {
			filePath := filepath.ToSlash(fileExt.FilePath)
			for _, sym := range fileExt.Symbols {
				if sym.Kind != KindTestFunction {
					continue
				}
				f := facts.TestFact{
					ID:            string(sym.ID),
					SymbolID:      string(sym.ID),
					FileID:        Stable("file", repoExt.Repository.RootPath, filePath),
					FilePath:      filePath,
					CanonicalName: sym.CanonicalName,
					Name:          sym.Name,
					StartLine:     sym.Location.StartLine,
					EndLine:       sym.Location.EndLine,
				}
				result = append(result, f)
			}
		}
	}
	return result
}

// ToIssueCountsFact converts inventory issue counts plus boundary detection issues to IssueCountsFact.
func ToIssueCountsFact(inventory Inventory, extraction ExtractionResult, boundaryResult BoundaryDetectionResult) facts.IssueCountsFact {
	counts := facts.IssueCountsFact{
		SkippedIgnoredFiles:           inventory.IssueCounts.SkippedIgnoredFiles,
		ServiceAttributionAmbiguities: inventory.IssueCounts.ServiceAttributionAmbiguities,
	}

	for _, repoExt := range extraction.Repositories {
		counts.UnresolvedImports += repoExt.Repository.IssueCounts.UnresolvedImports
		counts.AmbiguousRelations += repoExt.Repository.IssueCounts.AmbiguousRelations
		counts.UnsupportedConstructs += repoExt.Repository.IssueCounts.UnsupportedConstructs
	}

	// Boundary diagnostics count as deferred stitching issues
	for _, diag := range boundaryResult.Diagnostics {
		switch diag.Category {
		case "boundary_detection", "ambiguous_relation":
			counts.DeferredBoundaryStitching++
		default:
			counts.AmbiguousRelations++
		}
	}

	return counts
}

// BuildFactsIndex assembles all fact types from indexer outputs into a facts.BuildInput.
func BuildFactsIndex(
	inventory Inventory,
	extraction ExtractionResult,
	boundaryResult BoundaryDetectionResult,
	workspaceID string,
	snapshotID string,
) facts.BuildInput {
	return facts.BuildInput{
		WorkspaceID:    workspaceID,
		SnapshotID:     snapshotID,
		Repositories:   ToRepositoryFacts(inventory),
		Services:       ToServiceFacts(inventory),
		Files:          ToFileFacts(extraction),
		Symbols:        ToSymbolFacts(extraction),
		Imports:        ToImportFacts(extraction),
		Exports:        ToExportFacts(extraction),
		CallCandidates: ToCallCandidates(extraction),
		ExecutionHints: ToExecutionHints(extraction),
		Diagnostics:    append(ToDiagnosticFacts(extraction), ToBoundaryDiagnostics(boundaryResult)...),
		Tests:          ToTestFacts(extraction),
		Boundaries:     ToBoundaryFacts(boundaryResult),
		IssueCounts:    ToIssueCountsFact(inventory, extraction, boundaryResult),
	}
}

// BuildIndexFromExtraction is a convenience wrapper that calls BuildFactsIndex then facts.BuildIndex.
func BuildIndexFromExtraction(
	inventory Inventory,
	extraction ExtractionResult,
	boundaryResult BoundaryDetectionResult,
	workspaceID string,
	snapshotID string,
) facts.Index {
	input := BuildFactsIndex(inventory, extraction, boundaryResult, workspaceID, snapshotID)
	return facts.BuildIndex(input)
}

func init() {
	// Suppress unused import warning for fmt when needed by future enhancements.
	_ = fmt.Sprintf
}
