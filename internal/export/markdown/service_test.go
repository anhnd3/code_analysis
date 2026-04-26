package markdown

import (
	"strings"
	"testing"

	"analysis-module/internal/facts"
)

func TestRenderIncludesOnlyAcceptedSteps(t *testing.T) {
	body := New().Render(facts.ReviewFlow{
		ID:                "flow-1",
		RootCanonicalName: "demo.Root",
		Accepted: []facts.ReviewStep{
			{
				FromCanonicalName: "demo.Root",
				ToCanonicalName:   "demo.Child",
				Rationale:         "accepted path",
			},
		},
		Ambiguous: []facts.ReviewStep{
			{
				FromCanonicalName: "demo.Root",
				ToCanonicalName:   "demo.Unresolved",
				Rationale:         "needs evidence",
			},
		},
		Rejected: []facts.ReviewStep{
			{
				FromCanonicalName: "demo.Root",
				ToCanonicalName:   "demo.Test",
				Rationale:         "test edge",
			},
		},
		UncertaintyNotes: []string{"missing evidence"},
	})

	if !strings.Contains(body, "## Accepted Steps") || !strings.Contains(body, "demo.Root") || !strings.Contains(body, "demo.Child") {
		t.Fatalf("accepted steps missing from markdown: %s", body)
	}
	if strings.Contains(body, "## Ambiguous Steps") || strings.Contains(body, "## Rejected Steps") {
		t.Fatalf("markdown should not render ambiguous/rejected sections: %s", body)
	}
	if !strings.Contains(body, "## Uncertainty Notes") {
		t.Fatalf("uncertainty notes missing from markdown: %s", body)
	}
}
