package boundary

import (
	"fmt"
	"sort"

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

type registryEntry struct {
	Result
	key string
}

// dedupKey builds the normalized identity used to collapse duplicate detector output.
func dedupKey(root boundaryroot.Root) string {
	return fmt.Sprintf(
		"%s|%s|%d|%d|%s|%s|%s",
		root.RepositoryID,
		root.SourceFile,
		root.SourceStartByte,
		root.SourceEndByte,
		root.Method,
		root.Path,
		root.HandlerTarget,
	)
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

func resultOrderKey(result Result) string {
	root := result.Root
	return fmt.Sprintf(
		"%s|%s|%012d|%s|%s|%s|%s",
		root.RepositoryID,
		root.SourceFile,
		root.SourceStartByte,
		root.Method,
		root.Path,
		root.HandlerTarget,
		root.ID,
	)
}

func resultWins(candidate, existing registryEntry) bool {
	candidateRank := confidenceRank(candidate.Root.Confidence)
	existingRank := confidenceRank(existing.Root.Confidence)
	if candidateRank != existingRank {
		return candidateRank > existingRank
	}

	candidateHasTarget := candidate.Root.HandlerTarget != ""
	existingHasTarget := existing.Root.HandlerTarget != ""
	if candidateHasTarget != existingHasTarget {
		return candidateHasTarget
	}

	if candidate.Detector != existing.Detector {
		return candidate.Detector < existing.Detector
	}

	return candidate.Root.ID < existing.Root.ID
}

// DetectAll runs all registered detectors on the given file and returns de-duplicated results.
// Collisions are resolved deterministically so output ordering does not depend on detector
// registration order or map iteration order.
func (r *Registry) DetectAll(file ParsedGoFile, symbols []symbol.Symbol) []Result {
	// Collect raw results from all detectors.
	var raw []registryEntry
	for _, d := range r.detectors {
		roots, diags := d.DetectBoundaries(file, symbols)
		for _, root := range roots {
			raw = append(raw, registryEntry{
				Result: Result{
					Root:        root,
					Diagnostics: diags,
					Detector:    d.Name(),
				},
				key: dedupKey(root),
			})
		}
	}

	sort.Slice(raw, func(i, j int) bool {
		if raw[i].key != raw[j].key {
			return raw[i].key < raw[j].key
		}
		if raw[i].Detector != raw[j].Detector {
			return raw[i].Detector < raw[j].Detector
		}
		return raw[i].Root.ID < raw[j].Root.ID
	})

	// Dedup: keep one winner per key.
	winnersByKey := make(map[string]registryEntry, len(raw))
	diagsByKey := make(map[string][]symbol.Diagnostic, len(raw))

	for _, e := range raw {
		existing, seen := winnersByKey[e.key]
		if !seen {
			winnersByKey[e.key] = e
			continue
		}

		if resultWins(e, existing) {
			// New entry wins on confidence — replace.
			winnersByKey[e.key] = e
		} else if !resultWins(existing, e) {
			// Deterministic tie — emit ambiguity diagnostic for visibility.
			diagsByKey[e.key] = append(diagsByKey[e.key], symbol.Diagnostic{
				Category: "boundary_ambiguity",
				Message: fmt.Sprintf(
					"ambiguous boundary root for key %q: detectors %q and %q both reported it with confidence %q; keeping %q",
					e.key, existing.Detector, e.Detector, e.Root.Confidence, winnersByKey[e.key].Detector,
				),
				FilePath: e.Root.SourceFile,
			})
		}
		// If current entry loses deterministically, silently discard.
	}

	var out []Result
	for key, win := range winnersByKey {
		win.Diagnostics = append(win.Diagnostics, diagsByKey[key]...)
		out = append(out, win.Result)
	}

	sort.Slice(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		if resultOrderKey(left) != resultOrderKey(right) {
			return resultOrderKey(left) < resultOrderKey(right)
		}
		if left.Detector != right.Detector {
			return left.Detector < right.Detector
		}
		return left.Root.ID < right.Root.ID
	})

	return out
}
