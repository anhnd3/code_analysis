package symbol_index

import (
	"path/filepath"
	"strconv"

	"analysis-module/internal/app/progress"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	extractorport "analysis-module/internal/ports/extractor"
)

type RepositoryExtraction struct {
	Repository repository.Manifest           `json:"repository"`
	Files      []symbol.FileExtractionResult `json:"files"`
}

type Result struct {
	Repositories []RepositoryExtraction `json:"repositories"`
}

type Service struct {
	extractors []extractorport.SymbolExtractor
	reporter   progress.Reporter
}

func New(reporter progress.Reporter, extractors ...extractorport.SymbolExtractor) Service {
	if reporter == nil {
		reporter = progress.NoopReporter{}
	}
	return Service{extractors: extractors, reporter: reporter}
}

func (s Service) Build(inventory repository.Inventory) (Result, error) {
	repoByID := map[repository.ID]repository.Manifest{}
	for _, repo := range inventory.Repositories {
		repoByID[repo.ID] = repo
	}
	result := Result{}
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
			fileRef := symbol.FileRef{
				RepositoryID: string(repo.ID),
				AbsolutePath: filepath.Join(repo.RootPath, relPath),
				RelativePath: relPath,
				Language:     string(plan.Language),
			}
			extraction, err := extractor.ExtractFile(fileRef)
			if err != nil {
				s.reporter.FinishStage("extract failed")
				return Result{}, err
			}
			repoResult.Files = append(repoResult.Files, extraction)
			s.reporter.Advance(1)
		}
		result.Repositories = append(result.Repositories, repoResult)
	}
	s.reporter.FinishStage("repos=" + strconv.Itoa(len(result.Repositories)))
	return result, nil
}

func (s Service) findExtractor(lang string) extractorport.SymbolExtractor {
	for _, extractor := range s.extractors {
		if extractor.Supports(lang) {
			return extractor
		}
	}
	return nil
}
