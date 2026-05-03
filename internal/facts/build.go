package facts

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// BuildInput describes the inputs for building an Index.
// All fields use fact types only — no dependency on indexer package.
type BuildInput struct {
	WorkspaceID    string
	SnapshotID     string
	GeneratedAt    time.Time
	Repositories   []RepositoryFact
	Services       []ServiceFact
	Files          []FileFact
	Symbols        []SymbolFact
	Imports        []ImportFact
	Exports        []ExportFact
	CallCandidates []CallCandidate
	ExecutionHints []ExecutionHintFact
	Diagnostics    []DiagnosticFact
	Tests          []TestFact
	Boundaries     []BoundaryFact
	IssueCounts    IssueCountsFact
}

// BuildIndex assembles a complete Index from the given input.
func BuildIndex(input BuildInput) Index {
	if input.GeneratedAt.IsZero() {
		input.GeneratedAt = time.Now().UTC()
	}

	// Resolve call candidates to populate target_symbol_id where possible
	resolvedCandidates, resIssueCounts, resDiagnostics := ResolveCallCandidates(
		append([]CallCandidate{}, input.CallCandidates...), // copy to avoid mutating input
		input.Symbols,
		input.Exports,
		input.Files,
	)

	// Merge issue counts and diagnostics from resolution with original
	mergedIssueCounts := IssueCountsFact{
		SkippedIgnoredFiles:           input.IssueCounts.SkippedIgnoredFiles + resIssueCounts.SkippedIgnoredFiles,
		ServiceAttributionAmbiguities: input.IssueCounts.ServiceAttributionAmbiguities + resIssueCounts.ServiceAttributionAmbiguities,
		UnresolvedImports:             input.IssueCounts.UnresolvedImports + resIssueCounts.UnresolvedImports,
		AmbiguousRelations:            input.IssueCounts.AmbiguousRelations + resIssueCounts.AmbiguousRelations,
		UnsupportedConstructs:         input.IssueCounts.UnsupportedConstructs + resIssueCounts.UnsupportedConstructs,
		DeferredBoundaryStitching:     input.IssueCounts.DeferredBoundaryStitching + resIssueCounts.DeferredBoundaryStitching,
	}

	out := Index{
		WorkspaceID:    input.WorkspaceID,
		SnapshotID:     input.SnapshotID,
		GeneratedAt:    input.GeneratedAt,
		Repositories:   append([]RepositoryFact{}, input.Repositories...),
		Services:       append([]ServiceFact{}, input.Services...),
		Files:          append([]FileFact{}, input.Files...),
		Symbols:        append([]SymbolFact{}, input.Symbols...),
		Imports:        append([]ImportFact{}, input.Imports...),
		Exports:        append([]ExportFact{}, input.Exports...),
		CallCandidates: resolvedCandidates,
		ExecutionHints: append([]ExecutionHintFact{}, input.ExecutionHints...),
		Diagnostics:    append(append([]DiagnosticFact{}, input.Diagnostics...), resDiagnostics...),
		Tests:          append([]TestFact{}, input.Tests...),
		Boundaries:     append([]BoundaryFact{}, input.Boundaries...),
		IssueCounts:    mergedIssueCounts,
	}

	out.RepositoryCount = len(out.Repositories)
	out.FileCount = len(out.Files)
	out.SymbolCount = len(out.Symbols)
	out.CallCandidateCount = len(out.CallCandidates)
	return out
}

// resolveCandidateTarget attempts to resolve a call candidate target by file path and export name.
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

// ─── ID generation helpers ─────────────────────────────

func stableSuffix(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// StableID returns a short deterministic identifier derived from the provided parts,
// prefixed with the given prefix. Exported for use by other packages (review, etc.).
func StableID(prefix string, parts ...string) string {
	return prefix + "_" + stableSuffix(parts...)
}

// stableID is an alias kept for internal use within this package.
var stableID = StableID

// ResolveCallCandidates updates call candidates by resolving targets.
func ResolveCallCandidates(candidates []CallCandidate, symbols []SymbolFact, exports []ExportFact, files []FileFact) ([]CallCandidate, IssueCountsFact, []DiagnosticFact) {
	symbolByCanonical := map[string]string{}
	ambiguousCanonical := map[string]bool{}
	symbolByID := map[string]SymbolFact{}
	symbolByFile := map[string][]SymbolFact{}
	exportCanonicalByFile := map[string]map[string]string{}

	for _, sym := range symbols {
		symbolByID[sym.ID] = sym
		if existing, ok := symbolByCanonical[sym.CanonicalName]; ok && existing != sym.ID {
			ambiguousCanonical[sym.CanonicalName] = true
			delete(symbolByCanonical, sym.CanonicalName)
		} else if !ambiguousCanonical[sym.CanonicalName] {
			symbolByCanonical[sym.CanonicalName] = sym.ID
		}
		symbolByFile[filepath.ToSlash(sym.FilePath)] = append(symbolByFile[filepath.ToSlash(sym.FilePath)], sym)
	}

	for _, exp := range exports {
		fileKey := filepath.ToSlash(exp.FilePath)
		if exportCanonicalByFile[fileKey] == nil {
			exportCanonicalByFile[fileKey] = map[string]string{}
		}
		exportCanonicalByFile[fileKey][exp.Name] = exp.CanonicalName
	}

	issueCounts := IssueCountsFact{}
	diagnostics := make([]DiagnosticFact, 0)

	for i := range candidates {
		candidate := &candidates[i]
		if candidate.TargetSymbolID == "" && candidate.TargetCanonicalName != "" {
			if !ambiguousCanonical[candidate.TargetCanonicalName] {
				candidate.TargetSymbolID = symbolByCanonical[candidate.TargetCanonicalName]
			}
		}
		if candidate.TargetSymbolID == "" && candidate.TargetFilePath != "" && candidate.TargetExportName != "" {
			if targetID, resolved, ambiguous := resolveCandidateTarget(candidate.TargetFilePath, candidate.TargetExportName, symbolByFile, exportCanonicalByFile); resolved {
				candidate.TargetSymbolID = targetID
			} else if ambiguous {
				issueCounts.AmbiguousRelations++
				sourceFile := ""
				if source, ok := symbolByID[candidate.SourceSymbolID]; ok {
					sourceFile = source.FilePath
				}
				diagnostics = append(diagnostics, DiagnosticFact{
					ID:       stableID("fact", "diag", candidate.ID, "ambiguous_relation"),
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

	return candidates, issueCounts, diagnostics
}
