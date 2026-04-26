package extract

import (
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	"analysis-module/internal/services/boundary_detect"
	"analysis-module/internal/services/symbol_index"
)

type Result struct {
	Extraction symbol_index.Result `json:"extraction"`
	Boundaries []boundaryroot.Root `json:"boundaries"`
}

type Service struct {
	symbolIndex    symbol_index.Service
	boundaryDetect boundary_detect.Service
}

func New(symbolIndex symbol_index.Service, boundaryDetect boundary_detect.Service) Service {
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
