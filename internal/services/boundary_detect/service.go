package boundary_detect

import (
	"analysis-module/internal/adapters/boundary/go"
	"analysis-module/internal/adapters/extractor/treesitter"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Service provides high-level boundary detection across the workspace.
type Service struct {
	goRegistry *boundary.Registry
	goParser   *treesitter.GoParser
}

// New creates a new boundary detection service.
func New(goRegistry *boundary.Registry) Service {
	return Service{
		goRegistry: goRegistry,
		goParser:   treesitter.NewGoParser(),
	}
}

// DetectAll Go framework boundaries in the given repository inventory.
func (s Service) DetectAll(inventory repository.Inventory, symbols []symbol.Symbol) ([]boundaryroot.Root, error) {
	allRoots := []boundaryroot.Root{}

	for _, repo := range inventory.Repositories {
		// Detect if Go is in the tech stack
		hasGo := false
		for _, lang := range repo.TechStack.Languages {
			if lang == repository.LanguageGo {
				hasGo = true
				break
			}
		}

		if !hasGo {
			continue
		}

		roots, err := s.detectGoRepo(repo, symbols)
		if err != nil {
			return nil, err
		}
		allRoots = append(allRoots, roots...)
	}

	sortBoundaryRoots(allRoots)

	return allRoots, nil
}

func (s Service) detectGoRepo(repo repository.Manifest, allSymbols []symbol.Symbol) ([]boundaryroot.Root, error) {
	var roots []boundaryroot.Root

	for _, fileRelPath := range repo.GoFiles {
		absPath := filepath.Join(repo.RootPath, filepath.FromSlash(fileRelPath))

		// Optimization: only parse files that likely contain registrations
		// This can be refined, but for now we look for common framework keywords
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		if !isCandidateFile(content) {
			continue
		}

		tree, err := s.goParser.Parse(content)
		if err != nil {
			continue
		}
		defer tree.Close()

		file := boundary.ParsedGoFile{
			RepositoryID: string(repo.ID),
			Path:         fileRelPath,
			Content:      content,
			Root:         tree.RootNode(),
		}

		// Filter symbols for this file to be efficient
		var fileSymbols []symbol.Symbol
		for _, sym := range allSymbols {
			if string(sym.RepositoryID) == string(repo.ID) && (sym.FilePath == fileRelPath || strings.HasPrefix(fileRelPath, sym.FilePath)) {
				fileSymbols = append(fileSymbols, sym)
			}
		}

		results := s.goRegistry.DetectAll(file, fileSymbols)
		for _, res := range results {
			root := res.Root
			if root.RepositoryID == "" {
				root.RepositoryID = string(repo.ID)
			}
			if root.ID == "" {
				root.ID = boundaryroot.StableID(root)
			}
			roots = append(roots, root)
		}
	}

	sortBoundaryRoots(roots)

	return roots, nil
}

func isCandidateFile(content []byte) bool {
	s := string(content)
	// Keywords indicating router/server setup
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
