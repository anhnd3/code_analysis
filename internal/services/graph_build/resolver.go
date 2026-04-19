package graph_build

import (
	"strings"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/domain/targetref"
)

type resolutionBasis string

const (
	resolutionBasisExactSymbolID     resolutionBasis = "exact_symbol_id"
	resolutionBasisExactCanonical    resolutionBasis = "exact_canonical"
	resolutionBasisLocalExport       resolutionBasis = "local_export"
	resolutionBasisSameFileName      resolutionBasis = "same_file_name"
	resolutionBasisSamePackageName   resolutionBasis = "same_package_name"
	resolutionBasisRepoExactName     resolutionBasis = "repo_exact_name"
	resolutionBasisPackageMethodHint resolutionBasis = "package_method_hint"
)

type targetResolver struct {
	symbolByCanonical                 map[string]symbol.Symbol
	symbolByID                        map[symbol.ID]symbol.Symbol
	executableByFileAndName           map[string]map[string][]symbol.Symbol
	executableByPackageAndName        map[string]map[string][]symbol.Symbol
	executableByRepoAndName           map[string]map[string][]symbol.Symbol
	executableMethodsByPackageAndName map[string]map[string][]symbol.Symbol
	allMethodsByPackageAndName        map[string]map[string][]symbol.Symbol
	exportCanonicalByFile             map[string]map[string]string
	packageByFile                     map[string]string
}

func newTargetResolver(
	symbolByCanonical map[string]symbol.Symbol,
	symbolByID map[symbol.ID]symbol.Symbol,
	executableByFileAndName map[string]map[string][]symbol.Symbol,
	executableByPackageAndName map[string]map[string][]symbol.Symbol,
	executableByRepoAndName map[string]map[string][]symbol.Symbol,
	executableMethodsByPackageAndName map[string]map[string][]symbol.Symbol,
	allMethodsByPackageAndName map[string]map[string][]symbol.Symbol,
	exportCanonicalByFile map[string]map[string]string,
	packageByFile map[string]string,
) targetResolver {
	return targetResolver{
		symbolByCanonical:                 symbolByCanonical,
		symbolByID:                        symbolByID,
		executableByFileAndName:           executableByFileAndName,
		executableByPackageAndName:        executableByPackageAndName,
		executableByRepoAndName:           executableByRepoAndName,
		executableMethodsByPackageAndName: executableMethodsByPackageAndName,
		allMethodsByPackageAndName:        allMethodsByPackageAndName,
		exportCanonicalByFile:             exportCanonicalByFile,
		packageByFile:                     packageByFile,
	}
}

type targetLookupRequest struct {
	repoID         string
	sourceFile     string
	rawTarget      string
	targetKind     targetref.Kind
	targetFilePath string
	targetExport   string
}

func (r targetResolver) ResolveRelation(repoID, sourceFile string, relation symbol.RelationCandidate) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	req := targetLookupRequest{
		repoID:         repoID,
		sourceFile:     sourceFile,
		rawTarget:      relation.TargetCanonicalName,
		targetKind:     relation.TargetKind,
		targetFilePath: relation.TargetFilePath,
		targetExport:   relation.TargetExportName,
	}
	return r.resolve(req)
}

func (r targetResolver) ResolveBoundary(root boundaryroot.Root) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	req := targetLookupRequest{
		repoID:     root.RepositoryID,
		sourceFile: root.SourceFile,
		rawTarget:  root.HandlerTarget,
		targetKind: root.HandlerTargetKind,
	}
	if root.HandlerTargetKind == targetref.KindUnknown {
		if name := boundaryTargetName(root.HandlerTarget); name != "" {
			req.targetFilePath = root.SourceFile
			req.targetExport = name
		}
	}
	return r.resolve(req)
}

func (r targetResolver) resolve(req targetLookupRequest) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	raw := strings.TrimSpace(req.rawTarget)

	if raw != "" && (req.targetKind == targetref.KindExactSymbolID || req.targetKind == targetref.KindUnknown) {
		if sym, ok := r.symbolByID[symbol.ID(raw)]; ok && isExecutableSymbol(sym) {
			return sym, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.99}, resolutionBasisExactSymbolID, true
		}
	}

	if raw != "" && (req.targetKind == targetref.KindExactCanonical || req.targetKind == targetref.KindUnknown) {
		if sym, ok := r.symbolByCanonical[raw]; ok && isExecutableSymbol(sym) {
			return sym, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.95}, resolutionBasisExactCanonical, true
		}
	}

	if req.targetFilePath != "" && req.targetExport != "" {
		if exports := r.exportCanonicalByFile[req.targetFilePath]; exports != nil {
			if canonical := exports[req.targetExport]; canonical != "" {
				if sym, ok := r.symbolByCanonical[canonical]; ok && isExecutableSymbol(sym) {
					return sym, graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.93}, resolutionBasisLocalExport, true
				}
			}
		}
		if fileSymbols := r.executableByFileAndName[req.targetFilePath]; fileSymbols != nil {
			candidates := fileSymbols[req.targetExport]
			if len(candidates) == 1 {
				return candidates[0], graph.Confidence{Tier: graph.ConfidenceConfirmed, Score: 0.9}, resolutionBasisSameFileName, true
			}
		}
	}

	if req.targetKind == targetref.KindPackageMethodHint {
		return r.resolvePackageMethodHint(req.repoID, raw)
	}

	if raw == "" {
		return symbol.Symbol{}, graph.Confidence{}, "", false
	}

	if req.targetKind != targetref.KindUnknown {
		return symbol.Symbol{}, graph.Confidence{}, "", false
	}

	if pkgToken, methodName, ok := parsePackageMethodHint(raw); ok {
		if resolved, confidence, basis, ok := r.resolvePackageMethodHintWithParts(req.repoID, pkgToken, methodName); ok {
			return resolved, confidence, basis, true
		}
	}

	if isExactNameOnly(raw) {
		if packageName := r.packageByFile[req.sourceFile]; packageName != "" {
			candidates := r.executableByPackageAndName[packageSymbolsKey(req.repoID, packageName)][raw]
			if len(candidates) == 1 {
				return candidates[0], graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.8}, resolutionBasisSamePackageName, true
			}
		}
		if repoSymbols := r.executableByRepoAndName[req.repoID]; repoSymbols != nil {
			candidates := repoSymbols[raw]
			if len(candidates) == 1 {
				return candidates[0], graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.6}, resolutionBasisRepoExactName, true
			}
		}
	}

	return symbol.Symbol{}, graph.Confidence{}, "", false
}

func (r targetResolver) resolvePackageMethodHint(repoID, raw string) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	pkgToken, methodName, ok := parsePackageMethodHint(raw)
	if !ok {
		return symbol.Symbol{}, graph.Confidence{}, "", false
	}
	return r.resolvePackageMethodHintWithParts(repoID, pkgToken, methodName)
}

func (r targetResolver) resolvePackageMethodHintWithParts(repoID, pkgToken, methodName string) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	packageKey := packageSymbolsKey(repoID, pkgToken)
	candidates := append([]symbol.Symbol(nil), r.executableMethodsByPackageAndName[packageKey][methodName]...)
	candidates = filterPreferredMethodCandidates(candidates)
	if len(candidates) != 1 {
		return symbol.Symbol{}, graph.Confidence{}, "", false
	}
	return candidates[0], graph.Confidence{Tier: graph.ConfidenceInferred, Score: 0.82}, resolutionBasisPackageMethodHint, true
}

func filterPreferredMethodCandidates(candidates []symbol.Symbol) []symbol.Symbol {
	out := make([]symbol.Symbol, 0, len(candidates))
	for _, candidate := range candidates {
		if !isExecutableSymbol(candidate) {
			continue
		}
		if candidate.Kind != symbol.KindMethod || candidate.Receiver == "" {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func isExecutableSymbol(sym symbol.Symbol) bool {
	switch sym.Kind {
	case symbol.KindFunction, symbol.KindMethod, symbol.KindTestFunction, symbol.KindRouteHandler, symbol.KindGRPCHandler:
		return true
	default:
		return false
	}
}

func parsePackageMethodHint(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "/") {
		return "", "", false
	}
	if strings.Count(raw, ".") != 1 {
		return "", "", false
	}
	idx := strings.LastIndex(raw, ".")
	if idx <= 0 || idx >= len(raw)-1 {
		return "", "", false
	}
	return raw[:idx], raw[idx+1:], true
}

func isExactNameOnly(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw != "" && !strings.Contains(raw, ".") && !strings.Contains(raw, "/")
}
