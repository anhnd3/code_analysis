package indexer

// ParsedGoFile mimics the extractor's view of a Go file, providing necessary AST abstractions.
// Since the exact extractor AST type isn't globally exposed cleanly yet, we define a lightweight contract or use the raw file path.
// For now, we'll pass the file path, content, and the AST root node.

// BoundaryDetector is the generic adapter interface for finding framework-specific boundary registrations.
type BoundaryDetector interface {
	Name() string
	DetectBoundaries(file ParsedGoFile, symbols []Symbol) ([]BoundaryRoot, []Diagnostic)
}

// PackageAwareDetector is an optional capability for detectors that need
// package-scoped AST preparation before per-file boundary detection.
type PackageAwareDetector interface {
	PreparePackage(files []ParsedGoFile, symbols []Symbol) []Diagnostic
}
