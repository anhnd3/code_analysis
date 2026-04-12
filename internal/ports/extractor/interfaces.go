package extractor

import "analysis-module/internal/domain/symbol"

type SymbolExtractor interface {
	Supports(lang string) bool
	ExtractFile(file symbol.FileRef) (symbol.FileExtractionResult, error)
}

type LocalRelationExtractor interface {
	ExtractRelations(file symbol.FileRef, symbols []symbol.Symbol) ([]symbol.RelationCandidate, error)
}

type SemanticHarness interface {
	Name() string
}
