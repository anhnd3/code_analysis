package extract

import (
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/indexer/extract/boundaries"
	"analysis-module/internal/indexer/extract/symbols"
)

type Result struct {
	Extraction symbols.Result      `json:"extraction"`
	Boundaries []boundaryroot.Root `json:"boundaries"`
}

type Service struct {
	symbolIndex    symbols.Service
	boundaryDetect boundaries.Service
}

func New(symbolIndex symbols.Service, boundaryDetect boundaries.Service) Service {
	return Service{
		symbolIndex:    symbolIndex,
		boundaryDetect: boundaryDetect,
	}
}

func (s Service) Build(inventory repository.Inventory) (Result, error) {
	extraction, err := s.symbolIndex.Build(inventory)
	if err != nil {
		return Result{}, err
	}
	var allSymbols []symbol.Symbol
	for _, repoExt := range extraction.Repositories {
		for _, fileExt := range repoExt.Files {
			allSymbols = append(allSymbols, fileExt.Symbols...)
		}
	}
	boundaries, err := s.boundaryDetect.DetectAll(inventory, allSymbols)
	if err != nil {
		boundaries = nil
	}
	return Result{
		Extraction: extraction,
		Boundaries: boundaries,
	}, nil
}
