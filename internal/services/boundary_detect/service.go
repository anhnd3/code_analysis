package boundary_detect

import (
	boundary "analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Result keeps the internal orchestration split between recovered roots and
// structured diagnostics while preserving the external roots-only wrapper.
type Result struct {
	Roots       []boundaryroot.Root `json:"roots"`
	Diagnostics []symbol.Diagnostic `json:"diagnostics"`
}

type Service struct {
	goRegistry *boundary.Registry
	goParser   *treesitter.GoParser
}

type parsedGoFile struct {
	file        boundary.ParsedGoFile
	tree        *tree_sitter.Tree
	isCandidate bool
}

type parsedGoPackage struct {
	key       string
	files     []parsedGoFile
	hasTarget bool
}

func New(goRegistry *boundary.Registry) Service {
	return Service{
		goRegistry: goRegistry,
		goParser:   treesitter.NewGoParser(),
	}
}

// DetectAll preserves the previous roots-only service contract for callers
// that do not need diagnostics.
func (s Service) DetectAll(inventory repository.Inventory, symbols []symbol.Symbol) ([]boundaryroot.Root, error) {
	result, err := s.DetectAllDetailed(inventory, symbols)
	if err != nil {
		return nil, err
	}
	return result.Roots, nil
}

// DetectAllDetailed returns both detected roots and boundary diagnostics.
func (s Service) DetectAllDetailed(inventory repository.Inventory, symbols []symbol.Symbol) (Result, error) {
	out := Result{}

	for _, repo := range inventory.Repositories {
		if !repoHasLanguage(repo, repository.LanguageGo) {
			continue
		}

		repoResult, err := s.detectGoRepo(repo, symbols)
		if err != nil {
			return Result{}, err
		}
		out.Roots = append(out.Roots, repoResult.Roots...)
		out.Diagnostics = append(out.Diagnostics, repoResult.Diagnostics...)
	}

	sortBoundaryRoots(out.Roots)
	out.Diagnostics = sortAndDedupeDiagnostics(out.Diagnostics)
	return out, nil
}

func (s Service) detectGoRepo(repo repository.Manifest, allSymbols []symbol.Symbol) (Result, error) {
	packages, err := s.parseGoPackages(repo)
	if err != nil {
		return Result{}, err
	}
	defer closeParsedPackages(packages)

	symbolsByFile := symbolsByRepoFile(string(repo.ID), allSymbols)

	var result Result
	packageKeys := make([]string, 0, len(packages))
	for key := range packages {
		packageKeys = append(packageKeys, key)
	}
	sort.Strings(packageKeys)

	for _, key := range packageKeys {
		pkg := packages[key]
		files := make([]boundary.ParsedGoFile, 0, len(pkg.files))
		packageSymbols := make([]symbol.Symbol, 0)
		seenSymbols := map[string]symbol.Symbol{}
		for _, parsed := range pkg.files {
			files = append(files, parsed.file)
			for _, sym := range symbolsByFile[parsed.file.Path] {
				symKey := string(sym.ID)
				if symKey == "" {
					symKey = sym.FilePath + "|" + sym.CanonicalName
				}
				if _, exists := seenSymbols[symKey]; exists {
					continue
				}
				seenSymbols[symKey] = sym
				packageSymbols = append(packageSymbols, sym)
			}
		}

		result.Diagnostics = append(result.Diagnostics, s.goRegistry.PreparePackage(files, packageSymbols)...)
	}

	for _, key := range packageKeys {
		pkg := packages[key]
		if !pkg.hasTarget {
			continue
		}

		sort.Slice(pkg.files, func(i, j int) bool {
			return pkg.files[i].file.Path < pkg.files[j].file.Path
		})
		for _, parsed := range pkg.files {
			if !parsed.isCandidate {
				continue
			}

			fileSymbols := symbolsByFile[parsed.file.Path]
			detected, diags := s.goRegistry.DetectAllDetailed(parsed.file, fileSymbols)
			result.Diagnostics = append(result.Diagnostics, diags...)
			for _, entry := range detected {
				root := entry.Root
				if root.RepositoryID == "" {
					root.RepositoryID = string(repo.ID)
				}
				if root.ID == "" {
					root.ID = boundaryroot.StableID(root)
				}
				result.Roots = append(result.Roots, root)
			}
		}
	}

	sortBoundaryRoots(result.Roots)
	result.Diagnostics = sortAndDedupeDiagnostics(result.Diagnostics)
	return result, nil
}

func (s Service) parseGoPackages(repo repository.Manifest) (map[string]*parsedGoPackage, error) {
	packages := map[string]*parsedGoPackage{}

	for _, fileRelPath := range repo.GoFiles {
		absPath := filepath.Join(repo.RootPath, filepath.FromSlash(fileRelPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		tree, err := s.goParser.Parse(content)
		if err != nil {
			continue
		}

		parsed := boundary.ParsedGoFile{
			RepositoryID: string(repo.ID),
			Path:         fileRelPath,
			PackageName:  packageName(tree.RootNode(), content),
			Content:      content,
			Root:         tree.RootNode(),
		}
		key := packageGroupKey(parsed)
		group := packages[key]
		if group == nil {
			group = &parsedGoPackage{key: key}
			packages[key] = group
		}
		isCandidate := isCandidateFile(content)
		group.hasTarget = group.hasTarget || isCandidate
		group.files = append(group.files, parsedGoFile{
			file:        parsed,
			tree:        tree,
			isCandidate: isCandidate,
		})
	}

	return packages, nil
}

func repoHasLanguage(repo repository.Manifest, language repository.Language) bool {
	for _, candidate := range repo.TechStack.Languages {
		if candidate == language {
			return true
		}
	}
	return false
}

func closeParsedPackages(packages map[string]*parsedGoPackage) {
	for _, pkg := range packages {
		for _, file := range pkg.files {
			if file.tree != nil {
				file.tree.Close()
			}
		}
	}
}

func packageName(root *tree_sitter.Node, content []byte) string {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))
		if child == nil || child.Kind() != "package_clause" {
			continue
		}
		if name := child.ChildByFieldName("name"); name != nil {
			return string(content[name.StartByte():name.EndByte()])
		}
		if child.NamedChildCount() > 0 {
			name := child.NamedChild(0)
			return string(content[name.StartByte():name.EndByte()])
		}
	}
	return ""
}

func packageGroupKey(file boundary.ParsedGoFile) string {
	dir := filepath.ToSlash(filepath.Dir(file.Path))
	if dir == "." {
		dir = ""
	}
	return strings.Join([]string{file.RepositoryID, dir, file.PackageName}, "|")
}

func symbolsByRepoFile(repoID string, allSymbols []symbol.Symbol) map[string][]symbol.Symbol {
	grouped := map[string][]symbol.Symbol{}
	for _, sym := range allSymbols {
		if sym.RepositoryID != repoID {
			continue
		}
		grouped[sym.FilePath] = append(grouped[sym.FilePath], sym)
	}
	return grouped
}

func isCandidateFile(content []byte) bool {
	s := string(content)
	keywords := []string{"gin", "http", "mux", "HandleFunc", "Group", "grpc", "Register"}
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func sortBoundaryRoots(roots []boundaryroot.Root) {
	sort.Slice(roots, func(i, j int) bool {
		a := roots[i]
		b := roots[j]
		switch {
		case a.RepositoryID != b.RepositoryID:
			return a.RepositoryID < b.RepositoryID
		case a.SourceFile != b.SourceFile:
			return a.SourceFile < b.SourceFile
		case a.SourceStartByte != b.SourceStartByte:
			return a.SourceStartByte < b.SourceStartByte
		case a.Method != b.Method:
			return a.Method < b.Method
		case a.Path != b.Path:
			return a.Path < b.Path
		case a.HandlerTarget != b.HandlerTarget:
			return a.HandlerTarget < b.HandlerTarget
		default:
			return a.ID < b.ID
		}
	})
}

func diagnosticSortKey(diag symbol.Diagnostic) string {
	return strings.Join([]string{
		diag.FilePath,
		diag.Category,
		diag.Message,
		string(diag.SymbolID),
		diag.Evidence,
	}, "|")
}

func sortAndDedupeDiagnostics(diags []symbol.Diagnostic) []symbol.Diagnostic {
	seen := map[string]symbol.Diagnostic{}
	for _, diag := range diags {
		seen[diagnosticSortKey(diag)] = diag
	}

	result := make([]symbol.Diagnostic, 0, len(seen))
	for _, diag := range seen {
		result = append(result, diag)
	}
	sort.Slice(result, func(i, j int) bool {
		return diagnosticSortKey(result[i]) < diagnosticSortKey(result[j])
	})
	return result
}
