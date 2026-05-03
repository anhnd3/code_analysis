package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// BoundaryDetectionResult holds detected boundary roots and diagnostics.
type BoundaryDetectionResult struct {
	Roots       []BoundaryRoot `json:"roots"`
	Diagnostics []Diagnostic   `json:"diagnostics"`
}

// ParsedGoFile represents a parsed Go source file for boundary detection.
type ParsedGoFile struct {
	RepositoryID string
	Path         string
	PackageName  string
	Content      []byte
	Root         *tree_sitter.Node
}

// BoundaryDetectorService detects API boundaries using a registry of detectors.
type BoundaryDetectorService struct {
	registry *Registry
}

type parsedGoFile struct {
	file        ParsedGoFile
	tree        *tree_sitter.Tree
	isCandidate bool
}

type parsedGoPackage struct {
	key       string
	files     []parsedGoFile
	hasTarget bool
}

func NewBoundaryDetectorService(registry *Registry) BoundaryDetectorService {
	return BoundaryDetectorService{
		registry: registry,
	}
}

// DetectAll returns only the detected boundary roots (no diagnostics).
func (s BoundaryDetectorService) DetectAll(inventory Inventory, symbols []Symbol) ([]BoundaryRoot, error) {
	result, err := s.DetectAllDetailed(inventory, symbols)
	if err != nil {
		return nil, err
	}
	return result.Roots, nil
}

// DetectAllDetailed returns both detected roots and boundary diagnostics.
func (s BoundaryDetectorService) DetectAllDetailed(inventory Inventory, allSymbols []Symbol) (BoundaryDetectionResult, error) {
	out := BoundaryDetectionResult{}

	for _, repo := range inventory.Repositories {
		if !repoHasLanguage(repo, LanguageGo) {
			continue
		}

		repoResult, err := s.detectGoRepo(repo, allSymbols)
		if err != nil {
			return BoundaryDetectionResult{}, err
		}
		out.Roots = append(out.Roots, repoResult.Roots...)
		out.Diagnostics = append(out.Diagnostics, repoResult.Diagnostics...)
	}

	sortBoundaryRoots(out.Roots)
	out.Diagnostics = sortAndDedupeDiagnostics(out.Diagnostics)
	return out, nil
}

func (s BoundaryDetectorService) detectGoRepo(repo Manifest, allSymbols []Symbol) (BoundaryDetectionResult, error) {
	packages, err := s.parseGoPackages(repo)
	if err != nil {
		return BoundaryDetectionResult{}, err
	}
	defer closeParsedPackages(packages)

	symbolsByFile := symbolsByRepoFile(string(repo.ID), allSymbols)

	var result BoundaryDetectionResult
	packageKeys := make([]string, 0, len(packages))
	for key := range packages {
		packageKeys = append(packageKeys, key)
	}
	sort.Strings(packageKeys)

	for _, key := range packageKeys {
		pkg := packages[key]
		files := make([]ParsedGoFile, 0, len(pkg.files))
		packageSymbols := make([]Symbol, 0)
		seenSymbols := map[string]Symbol{}
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

		result.Diagnostics = append(result.Diagnostics, s.registry.PreparePackage(files, packageSymbols)...)
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
			detected, diags := s.registry.DetectAllDetailed(parsed.file, fileSymbols)
			result.Diagnostics = append(result.Diagnostics, diags...)
			for _, res := range detected {
				root := res.Root
				if root.RepositoryID == "" {
					root.RepositoryID = string(repo.ID)
				}
				if root.ID == "" {
					root.ID = StableBoundaryID(root)
				}
				result.Roots = append(result.Roots, root)
			}
		}
	}

	sortBoundaryRoots(result.Roots)
	result.Diagnostics = sortAndDedupeDiagnostics(result.Diagnostics)
	return result, nil
}

func (s BoundaryDetectorService) parseGoPackages(repo Manifest) (map[string]*parsedGoPackage, error) {
	packages := map[string]*parsedGoPackage{}

	for _, fileRelPath := range repo.GoFiles {
		absPath := filepath.Join(repo.RootPath, filepath.FromSlash(fileRelPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		tree, err := parseGoTree(content)
		if err != nil {
			continue
		}

		parsed := ParsedGoFile{
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

// parseGoTree parses Go source content into a tree-sitter tree.
func parseGoTree(content []byte) (*tree_sitter.Tree, error) {
	parser := NewGoParser()
	tree, err := parser.Parse(content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("failed to parse Go source")
	}
	return tree, nil
}

func repoHasLanguage(repo Manifest, language Language) bool {
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

func packageGroupKey(file ParsedGoFile) string {
	dir := filepath.ToSlash(filepath.Dir(file.Path))
	if dir == "." {
		dir = ""
	}
	return strings.Join([]string{file.RepositoryID, dir, file.PackageName}, "|")
}

func symbolsByRepoFile(repoID string, allSymbols []Symbol) map[string][]Symbol {
	grouped := map[string][]Symbol{}
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

func sortBoundaryRoots(roots []BoundaryRoot) {
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

func diagnosticSortKey(diag Diagnostic) string {
	return strings.Join([]string{
		diag.FilePath,
		diag.Category,
		diag.Message,
		string(diag.SymbolID),
		diag.Evidence,
	}, "|")
}

func sortAndDedupeDiagnostics(diags []Diagnostic) []Diagnostic {
	seen := map[string]Diagnostic{}
	for _, diag := range diags {
		seen[diagnosticSortKey(diag)] = diag
	}
	result := make([]Diagnostic, 0, len(seen))
	for _, diag := range seen {
		result = append(result, diag)
	}
	sort.Slice(result, func(i, j int) bool {
		return diagnosticSortKey(result[i]) < diagnosticSortKey(result[j])
	})
	return result
}
