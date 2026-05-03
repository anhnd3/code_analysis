package indexer

import (
	"path/filepath"
	"strconv"
)

// SymbolExtractorService builds symbol extraction results from a repository inventory.
type SymbolExtractorService struct {
	extractors []SymbolExtractor
	reporter   Reporter
}

func NewSymbolExtractorService(r Reporter, extractors ...SymbolExtractor) SymbolExtractorService {
	if r == nil {
		r = noopReporter{}
	}
	return SymbolExtractorService{extractors: extractors, reporter: r}
}

// Build extracts symbols from all files in the inventory.
func (s SymbolExtractorService) Build(inventory Inventory) (ExtractionResult, error) {
	repoByID := map[RepoID]Manifest{}
	for _, repo := range inventory.Repositories {
		repoByID[repo.ID] = repo
	}
	result := ExtractionResult{}
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
		repoResult := RepositoryExtraction{Repository: repo}
		for _, relPath := range plan.Files {
			s.reporter.Status("repo=" + repo.Name + " lang=" + string(plan.Language) + " file=" + relPath)
			fileRef := FileRef{
				RepositoryID:   string(repo.ID),
				RepositoryRoot: repo.RootPath,
				AbsolutePath:   filepath.Join(repo.RootPath, relPath),
				RelativePath:   relPath,
				Language:       string(plan.Language),
			}
			extraction, err := extractor.ExtractFile(fileRef)
			if err != nil {
				s.reporter.FinishStage("extract failed")
				return ExtractionResult{}, err
			}
			repoResult.Files = append(repoResult.Files, extraction)
			s.reporter.Advance(1)
		}
		result.Repositories = append(result.Repositories, repoResult)
	}
	s.reporter.FinishStage("repos=" + strconv.Itoa(len(result.Repositories)))
	return result, nil
}

func (s SymbolExtractorService) findExtractor(lang string) SymbolExtractor {
	for _, extractor := range s.extractors {
		if extractor.Supports(lang) {
			return extractor
		}
	}
	return nil
}
