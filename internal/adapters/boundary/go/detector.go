package boundary

import (
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// ParsedGoFile mimics the extractor's view of a Go file, providing necessary AST abstractions.
// Since the exact extractor AST type isn't globally exposed cleanly yet, we define a lightweight contract or use the raw file path.
// For now, we'll pass the file path, content, and the AST root node.
type ParsedGoFile struct {
	RepositoryID string
	Path         string
	PackageName  string
	Content      []byte
	Root         *tree_sitter.Node
}

// BoundaryDetector is the generic adapter interface for finding framework-specific boundary registrations.
type BoundaryDetector interface {
	Name() string
	DetectBoundaries(file ParsedGoFile, symbols []symbol.Symbol) ([]boundaryroot.Root, []symbol.Diagnostic)
}

// PackageAwareDetector is an optional capability for detectors that need
// package-scoped AST preparation before per-file boundary detection.
type PackageAwareDetector interface {
	PreparePackage(files []ParsedGoFile, symbols []symbol.Symbol) []symbol.Diagnostic
}
