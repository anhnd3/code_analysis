package boundary

import (
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/symbol"
)

// Registry manages the available framework boundary detectors.
type Registry struct {
	detectors []BoundaryDetector
}

// NewRegistry creates a new detector registry.
func NewRegistry() *Registry {
	return &Registry{
		detectors: []BoundaryDetector{},
	}
}

// Register adds a detector.
func (r *Registry) Register(detector BoundaryDetector) {
	r.detectors = append(r.detectors, detector)
}

// Result struct pairs the root with the detector that found it.
type Result struct {
	Root        boundaryroot.Root
	Diagnostics []symbol.Diagnostic
	Detector    string
}

// DetectAll runs all registered detectors on the given file.
func (r *Registry) DetectAll(file ParsedGoFile, symbols []symbol.Symbol) []Result {
	var allRoots []Result
	for _, d := range r.detectors {
		roots, diags := d.DetectBoundaries(file, symbols)
		for _, root := range roots {
			allRoots = append(allRoots, Result{
				Root:        root,
				Diagnostics: diags,
				Detector:    d.Name(),
			})
		}
	}
	// TODO: Phase 6 - Multiplexing & Deduplication logic should apply here or upstream when feeding entrypoint.
	return allRoots
}
