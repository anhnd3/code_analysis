package indexer

import (
	"path/filepath"
	"strconv"

	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	extractorport "analysis-module/internal/ports/extractor"
)

// SymbolExtractorService builds symbol extraction results from a repository inventory.
type SymbolExtractorService struct {
	extractors []extractorport.SymbolExtractor
	reporter   Reporter
}

func NewSymbolExtractorService(r Reporter, extractors ...extractorport.SymbolExtractor) SymbolExtractorService {
	if r == nil {
		r = noopReporter{}
	}
	return SymbolExtractorService{extractors: extractors, reporter: r}
}

// Build extracts symbols from all files in the inventory.
func (s SymbolExtractorService) Build(inventory repository.Inventory) (symbol.ExtractionResult, error) {
	repoByID := map[repository.ID]repository.Manifest{}
	for _, repo := range inventory.Repositories {
		repoByID[repo.ID] = repo
	}
	result := symbol.ExtractionResult{}
	totalFiles := 0
	for _, plan := range inventory.Plans {
		totalFiles += len(plan.Files)
	}
	s.reporter.StartStage("extract", totalFiles)
	for _, plan := range inventory.Plans {
		repo := repoByID[plan.RepositoryID]
		extractor := s.findExtractor(string(plan.Language))
		if extractor == nil {
			for range plan.Files {
				s.reporter.Advance(1)
			}
			continue
		}
		repoResult := symbol.RepositoryExtraction{Repository: repo}
		for _, relPath := range plan.Files {
			s.reporter.Status("repo=" + repo.Name + " lang=" + string(plan.Language) + " file=" + relPath)
			fileRef := symbol.FileRef{
				RepositoryID:   string(repo.ID),
				RepositoryRoot: repo.RootPath,
				AbsolutePath:   filepath.Join(repo.RootPath, relPath),
				RelativePath:   relPath,
				Language:       string(plan.Language),
			}
			extraction, err := extractor.ExtractFile(fileRef)
			if err != nil {
				s.reporter.FinishStage("extract failed")
				return symbol.ExtractionResult{}, err
			}
			repoResult.Files = append(repoResult.Files, extraction)
			s.reporter.Advance(1)
		}
		result.Repositories = append(result.Repositories, repoResult)
	}
	s.reporter.FinishStage("repos=" + strconv.Itoa(len(result.Repositories)))
	return result, nil
}

func (s SymbolExtractorService) findExtractor(lang string) extractorport.SymbolExtractor {
	for _, extractor := range s.extractors {
		if extractor.Supports(lang) {
			return extractor
		}
	}
	return nil
}
