package boundary

import (
	"fmt"

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

// dedupKey builds a composite key from the four dimensions that uniquely identify a boundary root.
func dedupKey(root boundaryroot.Root) string {
	return fmt.Sprintf("%s|%s|%s|%s", root.SourceFile, root.Method, root.Path, root.HandlerTarget)
}

// confidenceRank maps a confidence string to a numeric rank (higher = better).
func confidenceRank(c string) int {
	switch c {
	case "high":
		return 2
	case "medium":
		return 1
	default: // "low" or unknown
		return 0
	}
}

// DetectAll runs all registered detectors on the given file and returns de-duplicated results.
// Deduplication uses (SourceFile, Method, Path, HandlerTarget) as a composite key.
// For collisions, the root with the highest Confidence is kept. When confidence is equal,
// the first-seen root is kept and an ambiguity diagnostic is emitted.
func (r *Registry) DetectAll(file ParsedGoFile, symbols []symbol.Symbol) []Result {
	type entry struct {
		Result
		key string
	}

	// Collect raw results from all detectors.
	var raw []entry
	for _, d := range r.detectors {
		roots, diags := d.DetectBoundaries(file, symbols)
		for _, root := range roots {
			raw = append(raw, entry{
				Result: Result{
					Root:        root,
					Diagnostics: diags,
					Detector:    d.Name(),
				},
				key: dedupKey(root),
			})
		}
	}

	// Dedup: keep one winner per key.
	wInnersByKey := make(map[string]entry, len(raw))
	var diagsByKey []symbol.Diagnostic

	for _, e := range raw {
		existing, seen := wInnersByKey[e.key]
		if !seen {
			wInnersByKey[e.key] = e
			continue
		}

		currentRank := confidenceRank(e.Root.Confidence)
		existingRank := confidenceRank(existing.Root.Confidence)

		if currentRank > existingRank {
			// New entry wins on confidence — replace.
			wInnersByKey[e.key] = e
		} else if currentRank == existingRank {
			// Tie — emit ambiguity diagnostic, keep the first-seen.
			diagsByKey = append(diagsByKey, symbol.Diagnostic{
				Category: "boundary_ambiguity",
				Message: fmt.Sprintf(
					"ambiguous boundary root for key %q: detectors %q and %q both reported it with confidence %q; keeping %q",
					e.key, existing.Detector, e.Detector, e.Root.Confidence, existing.Detector,
				),
				FilePath: e.Root.SourceFile,
			})
		}
		// If current rank is lower, silently discard.
	}

	// Reconstruct ordered output (preserve insertion order via raw iteration).
	var out []Result
	seenKeys := make(map[string]bool, len(wInnersByKey))
	for _, e := range raw {
		if seenKeys[e.key] {
			continue
		}
		win := wInnersByKey[e.key]
		win.Diagnostics = append(win.Diagnostics, diagsByKey...)
		out = append(out, win.Result)
		seenKeys[e.key] = true
	}

	return out
}
