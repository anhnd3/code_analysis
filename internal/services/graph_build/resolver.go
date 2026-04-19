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

	if ref, ok := targetref.ParsePackageMethodHint(raw); ok {
		if resolved, confidence, basis, ok := r.resolvePackageMethodHintWithParts(req.repoID, ref.PackageToken, ref.MethodName); ok {
			return resolved, confidence, basis, true
		}
	}

	return symbol.Symbol{}, graph.Confidence{}, "", false
}

func (r targetResolver) resolvePackageMethodHint(repoID, raw string) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	ref, ok := targetref.ParsePackageMethodHint(raw)
	if !ok {
		return symbol.Symbol{}, graph.Confidence{}, "", false
	}
	return r.resolvePackageMethodHintWithParts(repoID, ref.PackageToken, ref.MethodName)
}

func (r targetResolver) resolvePackageMethodHintWithParts(repoID, pkgToken, methodName string) (symbol.Symbol, graph.Confidence, resolutionBasis, bool) {
	packageKey := packageSymbolsKey(repoID, targetref.NormalizePackageToken(pkgToken))
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
