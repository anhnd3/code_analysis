package mermaid

import (
	"strings"
	"testing"

	"analysis-module/internal/facts"
)

func TestRenderIncludesOnlyAcceptedSteps(t *testing.T) {
	diagram := New().Render(facts.ReviewFlow{
		RootSymbolID:      "sym-root",
		RootCanonicalName: "demo.Root",
		Accepted: []facts.ReviewStep{
			{
				FromSymbolID:      "sym-root",
				FromCanonicalName: "demo.Root",
				ToSymbolID:        "sym-child",
				ToCanonicalName:   "demo.Child",
				Rationale:         "accepted path",
			},
		},
		Rejected: []facts.ReviewStep{
			{
				FromSymbolID:      "sym-root",
				FromCanonicalName: "demo.Root",
				ToSymbolID:        "sym-test",
				ToCanonicalName:   "demo.Test",
				Rationale:         "rejected path",
			},
		},
	})

	if !strings.Contains(diagram, "sequenceDiagram") || !strings.Contains(diagram, "Root") || !strings.Contains(diagram, "Child") {
		t.Fatalf("accepted mermaid edge missing: %s", diagram)
	}
	if strings.Contains(diagram, "demo.Test") || strings.Contains(diagram, "rejected path") {
		t.Fatalf("mermaid renderer should omit rejected steps: %s", diagram)
	}
}
