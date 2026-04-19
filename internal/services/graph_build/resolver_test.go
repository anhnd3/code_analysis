package graph_build

import (
	"testing"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/domain/targetref"
)

func TestTargetResolverResolvesExactSymbolID(t *testing.T) {
	target := testSymbol("sym_target", "repo1", "handler.go", "demo", "", "Handle", symbol.KindFunction)
	resolver := testTargetResolver(target)

	resolved, confidence, basis, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetCanonicalName: string(target.ID),
		TargetKind:          targetref.KindExactSymbolID,
	})
	if !ok {
		t.Fatal("expected exact symbol ID to resolve")
	}
	if resolved.ID != target.ID {
		t.Fatalf("expected %s, got %+v", target.ID, resolved)
	}
	if basis != resolutionBasisExactSymbolID {
		t.Fatalf("expected exact symbol ID basis, got %s", basis)
	}
	if confidence.Tier != "confirmed" {
		t.Fatalf("expected confirmed confidence, got %+v", confidence)
	}
}

func TestTargetResolverResolvesExactCanonical(t *testing.T) {
	target := testSymbol("sym_target", "repo1", "handler.go", "demo", "handler", "Handle", symbol.KindMethod)
	resolver := testTargetResolver(target)

	resolved, confidence, basis, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetCanonicalName: target.CanonicalName,
		TargetKind:          targetref.KindExactCanonical,
	})
	if !ok {
		t.Fatal("expected exact canonical to resolve")
	}
	if resolved.CanonicalName != target.CanonicalName {
		t.Fatalf("expected %s, got %+v", target.CanonicalName, resolved)
	}
	if basis != resolutionBasisExactCanonical {
		t.Fatalf("expected exact canonical basis, got %s", basis)
	}
	if confidence.Tier != "confirmed" {
		t.Fatalf("expected confirmed confidence, got %+v", confidence)
	}
}

func TestTargetResolverResolvesUniquePackageMethodHint(t *testing.T) {
	target := testSymbol("sym_repo", "repo1", "repo.go", "camerarepo", "cameraRepo", "DetectQR", symbol.KindMethod)
	resolver := testTargetResolver(target)

	resolved, confidence, basis, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetCanonicalName: "camerarepo.DetectQR",
		TargetKind:          targetref.KindPackageMethodHint,
	})
	if !ok {
		t.Fatal("expected unique package method hint to resolve")
	}
	if resolved.CanonicalName != target.CanonicalName {
		t.Fatalf("expected %s, got %+v", target.CanonicalName, resolved)
	}
	if basis != resolutionBasisPackageMethodHint {
		t.Fatalf("expected package hint basis, got %s", basis)
	}
	if confidence.Tier != "inferred" {
		t.Fatalf("expected inferred confidence, got %+v", confidence)
	}
}

func TestTargetResolverKeepsPackageTokenExactAcrossPackages(t *testing.T) {
	target := testSymbol("sym_repo_a", "repo1", "repo_a.go", "camerarepo", "cameraRepo", "DetectQR", symbol.KindMethod)
	other := testSymbol("sym_repo_b", "repo1", "repo_b.go", "repository_v2", "cameraV2Repository", "DetectQR", symbol.KindMethod)
	resolver := testTargetResolver(target, other)

	resolved, _, _, ok := resolver.ResolveBoundary(boundaryroot.Root{
		RepositoryID:      "repo1",
		SourceFile:        "routes.go",
		HandlerTarget:     "camerarepo.DetectQR",
		HandlerTargetKind: targetref.KindPackageMethodHint,
	})
	if !ok {
		t.Fatal("expected exact package token hint to resolve")
	}
	if resolved.CanonicalName != target.CanonicalName {
		t.Fatalf("expected %s, got %+v", target.CanonicalName, resolved)
	}
}

func TestTargetResolverDoesNotGuessAcrossReceiverFamilies(t *testing.T) {
	left := testSymbol("sym_repo_a", "repo1", "repo_a.go", "camerarepo", "cameraRepo", "DetectQR", symbol.KindMethod)
	right := testSymbol("sym_repo_b", "repo1", "repo_b.go", "camerarepo", "cameraRepoV2", "DetectQR", symbol.KindMethod)
	resolver := testTargetResolver(left, right)

	_, _, _, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetCanonicalName: "camerarepo.DetectQR",
		TargetKind:          targetref.KindPackageMethodHint,
	})
	if ok {
		t.Fatal("expected ambiguous same-package receiver families to stay unresolved")
	}
}

func TestTargetResolverPrefersConcreteMethodsOverInterfaceSymbols(t *testing.T) {
	concrete := testSymbol("sym_repo_a", "repo1", "repo.go", "session", "sessionService", "GetSessionByZlpToken", symbol.KindMethod)
	iface := testSymbol("sym_iface", "repo1", "types.go", "session", "", "GetSessionByZlpToken", symbol.KindInterface)
	resolver := testTargetResolver(concrete, iface)

	resolved, _, _, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetCanonicalName: "session.GetSessionByZlpToken",
		TargetKind:          targetref.KindPackageMethodHint,
	})
	if !ok {
		t.Fatal("expected package hint to ignore non-executable interface symbols")
	}
	if resolved.CanonicalName != concrete.CanonicalName {
		t.Fatalf("expected %s, got %+v", concrete.CanonicalName, resolved)
	}
}

func TestTargetResolverResolvesExtractorProvidedExportMetadata(t *testing.T) {
	target := testSymbol("sym_repo", "repo1", "repo.go", "camerarepo", "cameraRepo", "DetectQR", symbol.KindMethod)
	resolver := testTargetResolver(target)
	resolver.exportCanonicalByFile["repo.go"] = map[string]string{
		"DetectQR": target.CanonicalName,
	}

	resolved, confidence, basis, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetFilePath:   "repo.go",
		TargetExportName: "DetectQR",
	})
	if !ok {
		t.Fatal("expected extractor-provided file/export metadata to resolve")
	}
	if resolved.CanonicalName != target.CanonicalName {
		t.Fatalf("expected %s, got %+v", target.CanonicalName, resolved)
	}
	if basis != resolutionBasisLocalExport {
		t.Fatalf("expected local export basis, got %s", basis)
	}
	if confidence.Tier != "confirmed" {
		t.Fatalf("expected confirmed confidence, got %+v", confidence)
	}
}

func TestTargetResolverDoesNotGuessExactNameOnly(t *testing.T) {
	target := testSymbol("sym_repo", "repo1", "repo.go", "camerarepo", "cameraRepo", "DetectQR", symbol.KindMethod)
	resolver := testTargetResolver(target)

	_, _, _, ok := resolver.ResolveRelation("repo1", "handler.go", symbol.RelationCandidate{
		TargetCanonicalName: "DetectQR",
	})
	if ok {
		t.Fatal("expected exact-name-only target to stay unresolved")
	}
}

func testSymbol(id, repoID, filePath, pkg, receiver, name string, kind symbol.Kind) symbol.Symbol {
	return symbol.Symbol{
		ID:            symbol.ID(id),
		RepositoryID:  repoID,
		FilePath:      filePath,
		PackageName:   pkg,
		Receiver:      receiver,
		Name:          name,
		CanonicalName: testCanonicalName(pkg, receiver, name),
		Kind:          kind,
	}
}

func testCanonicalName(pkg, receiver, name string) string {
	if receiver != "" {
		return pkg + "." + receiver + "." + name
	}
	return pkg + "." + name
}

func testTargetResolver(symbols ...symbol.Symbol) targetResolver {
	symbolByCanonical := map[string]symbol.Symbol{}
	symbolByID := map[symbol.ID]symbol.Symbol{}
	executableByFileAndName := map[string]map[string][]symbol.Symbol{}
	executableByPackageAndName := map[string]map[string][]symbol.Symbol{}
	executableByRepoAndName := map[string]map[string][]symbol.Symbol{}
	executableMethodsByPackageAndName := map[string]map[string][]symbol.Symbol{}
	allMethodsByPackageAndName := map[string]map[string][]symbol.Symbol{}
	exportCanonicalByFile := map[string]map[string]string{}
	packageByFile := map[string]string{}

	for _, sym := range symbols {
		symbolByCanonical[sym.CanonicalName] = sym
		symbolByID[sym.ID] = sym

		if executableByFileAndName[sym.FilePath] == nil {
			executableByFileAndName[sym.FilePath] = map[string][]symbol.Symbol{}
		}
		packageKey := packageSymbolsKey(sym.RepositoryID, sym.PackageName)
		if executableByPackageAndName[packageKey] == nil {
			executableByPackageAndName[packageKey] = map[string][]symbol.Symbol{}
		}
		if executableMethodsByPackageAndName[packageKey] == nil {
			executableMethodsByPackageAndName[packageKey] = map[string][]symbol.Symbol{}
		}
		if allMethodsByPackageAndName[packageKey] == nil {
			allMethodsByPackageAndName[packageKey] = map[string][]symbol.Symbol{}
		}
		if executableByRepoAndName[sym.RepositoryID] == nil {
			executableByRepoAndName[sym.RepositoryID] = map[string][]symbol.Symbol{}
		}
		packageByFile[sym.FilePath] = sym.PackageName

		if isExecutableSymbol(sym) {
			executableByFileAndName[sym.FilePath][sym.Name] = append(executableByFileAndName[sym.FilePath][sym.Name], sym)
			executableByPackageAndName[packageKey][sym.Name] = append(executableByPackageAndName[packageKey][sym.Name], sym)
			executableByRepoAndName[sym.RepositoryID][sym.Name] = append(executableByRepoAndName[sym.RepositoryID][sym.Name], sym)
		}
		if sym.Kind == symbol.KindMethod {
			allMethodsByPackageAndName[packageKey][sym.Name] = append(allMethodsByPackageAndName[packageKey][sym.Name], sym)
			if isExecutableSymbol(sym) {
				executableMethodsByPackageAndName[packageKey][sym.Name] = append(executableMethodsByPackageAndName[packageKey][sym.Name], sym)
			}
		}
	}

	return newTargetResolver(
		symbolByCanonical,
		symbolByID,
		executableByFileAndName,
		executableByPackageAndName,
		executableByRepoAndName,
		executableMethodsByPackageAndName,
		allMethodsByPackageAndName,
		exportCanonicalByFile,
		packageByFile,
	)
}
