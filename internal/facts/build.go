package facts

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/symbol_index"
	"analysis-module/pkg/ids"
)

type BuildInput struct {
	WorkspaceID string
	SnapshotID  string
	Inventory   repository.Inventory
	Extraction  symbol_index.Result
	Boundaries  []boundaryroot.Root
	GeneratedAt time.Time
}

func BuildIndex(input BuildInput) Index {
	if input.GeneratedAt.IsZero() {
		input.GeneratedAt = time.Now().UTC()
	}
	out := Index{
		WorkspaceID: input.WorkspaceID,
		SnapshotID:  input.SnapshotID,
		GeneratedAt: input.GeneratedAt,
		IssueCounts: toIssueCounts(input.Inventory.IssueCounts),
	}

	symbolByCanonical := map[string]string{}
	ambiguousCanonical := map[string]bool{}
	symbolByID := map[string]SymbolFact{}
	symbolByFile := map[string][]SymbolFact{}
	exportCanonicalByFile := map[string]map[string]string{}
	repoByID := map[string]repository.Manifest{}
	for _, repo := range input.Inventory.Repositories {
		repoByID[string(repo.ID)] = repo
		out.Repositories = append(out.Repositories, RepositoryFact{
			ID:         string(repo.ID),
			Name:       repo.Name,
			RootPath:   repo.RootPath,
			Role:       string(repo.Role),
			Languages:  stringSlice(repo.TechStack.Languages),
			BuildFiles: append([]string{}, repo.TechStack.BuildFiles...),
			Frameworks: append([]string{}, repo.TechStack.FrameworkHints...),
		})
		for _, svc := range repo.CandidateServices {
			hints := make([]string, 0, len(svc.Boundaries))
			for _, hint := range svc.Boundaries {
				hints = append(hints, string(hint))
			}
			out.Services = append(out.Services, ServiceFact{
				ID:            string(svc.ID),
				RepositoryID:  string(repo.ID),
				Name:          svc.Name,
				RootPath:      svc.RootPath,
				Entrypoints:   append([]string{}, svc.Entrypoints...),
				BoundaryHints: hints,
			})
		}
	}

	for _, repoExtraction := range input.Extraction.Repositories {
		repo := repoByID[string(repoExtraction.Repository.ID)]
		for _, file := range repoExtraction.Files {
			fileID := ids.Stable("fact", "file", string(repo.ID), file.FilePath)
			fileFact := FileFact{
				ID:             fileID,
				RepositoryID:   string(repo.ID),
				RepositoryRoot: repo.RootPath,
				RelativePath:   filepath.ToSlash(file.FilePath),
				AbsolutePath:   filepath.Join(repo.RootPath, file.FilePath),
				Language:       file.Language,
				PackageName:    file.PackageName,
			}
			out.Files = append(out.Files, fileFact)

			for _, imp := range file.ImportBindings {
				out.Imports = append(out.Imports, ImportFact{
					ID:           ids.Stable("fact", "import", fileID, imp.Source, imp.Alias, imp.ImportedName),
					FileID:       fileID,
					FilePath:     file.FilePath,
					Source:       imp.Source,
					Alias:        imp.Alias,
					ImportedName: imp.ImportedName,
					ExportName:   imp.ExportName,
					ResolvedPath: imp.ResolvedPath,
					IsDefault:    imp.IsDefault,
					IsNamespace:  imp.IsNamespace,
					IsLocal:      imp.IsLocal,
				})
			}
			if len(file.ImportBindings) == 0 {
				for _, imported := range file.Imports {
					out.Imports = append(out.Imports, ImportFact{
						ID:       ids.Stable("fact", "import", fileID, imported),
						FileID:   fileID,
						FilePath: file.FilePath,
						Source:   imported,
					})
				}
			}

			for _, exp := range file.Exports {
				fileKey := filepath.ToSlash(file.FilePath)
				if exportCanonicalByFile[fileKey] == nil {
					exportCanonicalByFile[fileKey] = map[string]string{}
				}
				exportCanonicalByFile[fileKey][exp.Name] = exp.CanonicalName
				out.Exports = append(out.Exports, ExportFact{
					ID:            ids.Stable("fact", "export", fileID, exp.Name, exp.CanonicalName),
					FileID:        fileID,
					FilePath:      file.FilePath,
					Name:          exp.Name,
					CanonicalName: exp.CanonicalName,
					IsDefault:     exp.IsDefault,
				})
			}

			for _, diag := range file.Diagnostics {
				out.Diagnostics = append(out.Diagnostics, DiagnosticFact{
					ID:       ids.Stable("fact", "diag", fileID, diag.Category, diag.Message, string(diag.SymbolID)),
					FilePath: file.FilePath,
					SymbolID: string(diag.SymbolID),
					Category: diag.Category,
					Message:  diag.Message,
					Evidence: diag.Evidence,
				})
			}

			for _, sym := range file.Symbols {
				symbolFact := SymbolFact{
					ID:            string(sym.ID),
					RepositoryID:  sym.RepositoryID,
					FileID:        fileID,
					FilePath:      sym.FilePath,
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
				out.Symbols = append(out.Symbols, symbolFact)
				symbolByID[string(sym.ID)] = symbolFact
				if existing, ok := symbolByCanonical[sym.CanonicalName]; ok && existing != string(sym.ID) {
					ambiguousCanonical[sym.CanonicalName] = true
					delete(symbolByCanonical, sym.CanonicalName)
				} else if !ambiguousCanonical[sym.CanonicalName] {
					symbolByCanonical[sym.CanonicalName] = string(sym.ID)
				}
				symbolByFile[filepath.ToSlash(sym.FilePath)] = append(symbolByFile[filepath.ToSlash(sym.FilePath)], symbolFact)
				if sym.Kind == symbol.KindTestFunction {
					out.Tests = append(out.Tests, TestFact{
						ID:            ids.Stable("fact", "test", string(sym.ID)),
						SymbolID:      string(sym.ID),
						FileID:        fileID,
						FilePath:      sym.FilePath,
						CanonicalName: sym.CanonicalName,
						Name:          sym.Name,
						StartLine:     sym.Location.StartLine,
						EndLine:       sym.Location.EndLine,
					})
				}
			}

			for _, relation := range file.Relations {
				targetID := symbolByCanonical[relation.TargetCanonicalName]
				out.CallCandidates = append(out.CallCandidates, CallCandidate{
					ID:                  ids.Stable("fact", "call", string(relation.SourceSymbolID), relation.TargetCanonicalName, relation.EvidenceSource, strconv.Itoa(relation.OrderIndex)),
					SourceSymbolID:      string(relation.SourceSymbolID),
					TargetSymbolID:      targetID,
					TargetCanonicalName: relation.TargetCanonicalName,
					TargetFilePath:      relation.TargetFilePath,
					TargetExportName:    relation.TargetExportName,
					Relationship:        relation.Relationship,
					EvidenceType:        relation.EvidenceType,
					EvidenceSource:      relation.EvidenceSource,
					ExtractionMethod:    relation.ExtractionMethod,
					ConfidenceScore:     relation.ConfidenceScore,
					OrderIndex:          relation.OrderIndex,
				})
			}

			for _, hint := range file.Hints {
				out.ExecutionHints = append(out.ExecutionHints, ExecutionHintFact{
					ID:             ids.Stable("fact", "hint", hint.SourceSymbolID, string(hint.Kind), hint.Evidence, strconv.Itoa(hint.OrderIndex)),
					FilePath:       file.FilePath,
					SourceSymbolID: hint.SourceSymbolID,
					TargetSymbolID: hint.TargetSymbolID,
					TargetSymbol:   hint.TargetSymbol,
					Kind:           string(hint.Kind),
					Evidence:       hint.Evidence,
					OrderIndex:     hint.OrderIndex,
				})
			}
		}
	}

	for _, root := range input.Boundaries {
		out.Boundaries = append(out.Boundaries, BoundaryFact{
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
		})
	}

	for i := range out.CallCandidates {
		candidate := &out.CallCandidates[i]
		if candidate.TargetSymbolID == "" && candidate.TargetCanonicalName != "" {
			if !ambiguousCanonical[candidate.TargetCanonicalName] {
				candidate.TargetSymbolID = symbolByCanonical[candidate.TargetCanonicalName]
			}
		}
		if candidate.TargetSymbolID == "" && candidate.TargetFilePath != "" && candidate.TargetExportName != "" {
			if targetID, resolved, ambiguous := resolveCandidateTarget(candidate.TargetFilePath, candidate.TargetExportName, symbolByFile, exportCanonicalByFile); resolved {
				candidate.TargetSymbolID = targetID
			} else if ambiguous {
				out.IssueCounts.AmbiguousRelations++
				sourceFile := ""
				if source, ok := symbolByID[candidate.SourceSymbolID]; ok {
					sourceFile = source.FilePath
				}
				out.Diagnostics = append(out.Diagnostics, DiagnosticFact{
					ID:       ids.Stable("fact", "diag", candidate.ID, "ambiguous_relation"),
					FilePath: sourceFile,
					Category: "ambiguous_relation",
					Message:  fmt.Sprintf("could not uniquely resolve %q in %q", candidate.TargetExportName, candidate.TargetFilePath),
					Evidence: candidate.EvidenceSource,
				})
			}
		}
		if candidate.TargetFilePath == "" && candidate.TargetSymbolID != "" {
			if sym, ok := symbolByID[candidate.TargetSymbolID]; ok {
				candidate.TargetFilePath = sym.FilePath
			}
		}
	}

	out.RepositoryCount = len(out.Repositories)
	out.FileCount = len(out.Files)
	out.SymbolCount = len(out.Symbols)
	out.CallCandidateCount = len(out.CallCandidates)
	return out
}

func resolveCandidateTarget(targetFilePath, targetExportName string, symbolByFile map[string][]SymbolFact, exportCanonicalByFile map[string]map[string]string) (string, bool, bool) {
	fileKey := filepath.ToSlash(targetFilePath)
	matches := make([]SymbolFact, 0)
	if exports := exportCanonicalByFile[fileKey]; exports != nil {
		if canonical := exports[targetExportName]; canonical != "" {
			for _, sym := range symbolByFile[fileKey] {
				if sym.CanonicalName == canonical {
					matches = append(matches, sym)
				}
			}
			if len(matches) == 1 {
				return matches[0].ID, true, false
			}
		}
	}
	for _, sym := range symbolByFile[fileKey] {
		if sym.Name == targetExportName || strings.HasSuffix(sym.CanonicalName, "."+targetExportName) {
			matches = append(matches, sym)
		}
	}
	if len(matches) == 1 {
		return matches[0].ID, true, false
	}
	if len(matches) > 1 {
		return "", false, true
	}
	return "", false, true
}

func toIssueCounts(in analysis.IssueCounts) IssueCountsFact {
	return IssueCountsFact{
		UnresolvedImports:             in.UnresolvedImports,
		AmbiguousRelations:            in.AmbiguousRelations,
		UnsupportedConstructs:         in.UnsupportedConstructs,
		SkippedIgnoredFiles:           in.SkippedIgnoredFiles,
		DeferredBoundaryStitching:     in.DeferredBoundaryStitching,
		ServiceAttributionAmbiguities: in.ServiceAttributionAmbiguities,
	}
}

func stringSlice[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
