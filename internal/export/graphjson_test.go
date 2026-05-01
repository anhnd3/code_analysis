package export

import (
	"encoding/json"
	"testing"

	"analysis-module/internal/facts"
)

func TestGraphJSONAcceptedEdgeAppears(t *testing.T) {
	flow := facts.ReviewFlow{
		ID:                "flow-1",
		RootSymbolID:      "sym-root",
		RootCanonicalName: "demo.Root",
		Accepted: []facts.ReviewStep{
			{FromSymbolID: "sym-root", FromCanonicalName: "demo.Root", ToSymbolID: "sym-child", ToCanonicalName: "demo.Child"},
		},
	}

	svc := NewGraphJSONService()
	data, err := svc.Render(flow)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var result GraphJSON
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Edges) != 1 {
		t.Fatalf("expected one accepted edge, got %d", len(result.Edges))
	}
	if result.Edges[0].From != "sym-root" || result.Edges[0].To != "sym-child" {
		t.Errorf("unexpected edge: %+v", result.Edges[0])
	}
}

func TestGraphJSONRootAndTargetNodesAppearOnce(t *testing.T) {
	flow := facts.ReviewFlow{
		ID:                "flow-1",
		RootSymbolID:      "sym-root",
		RootCanonicalName: "demo.Root",
		Accepted: []facts.ReviewStep{
			{FromSymbolID: "sym-root", FromCanonicalName: "demo.Root", ToSymbolID: "sym-child", ToCanonicalName: "demo.Child"},
			{FromSymbolID: "sym-child", FromCanonicalName: "demo.Child", ToSymbolID: "sym-grand", ToCanonicalName: "demo.Grand"},
		},
	}

	svc := NewGraphJSONService()
	data, err := svc.Render(flow)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var result GraphJSON
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	counts := make(map[string]int)
	for _, n := range result.Nodes {
		counts[n.Id]++
	}

	if counts["sym-root"] > 1 || counts["sym-child"] > 1 || counts["sym-grand"] > 1 {
		t.Errorf("nodes should appear once; got: %+v", counts)
	}

	if len(result.Nodes) != 3 {
		t.Fatalf("expected exactly three nodes (root, child, grand), got %d", len(result.Nodes))
	}
}

func TestGraphJSONRejectedAndAmbiguousStepsAbsent(t *testing.T) {
	flow := facts.ReviewFlow{
		ID:                "flow-1",
		RootSymbolID:      "sym-root",
		RootCanonicalName: "demo.Root",
		Accepted: []facts.ReviewStep{
			{FromSymbolID: "sym-root", FromCanonicalName: "demo.Root", ToSymbolID: "sym-child", ToCanonicalName: "demo.Child"},
		},
		Ambiguous: []facts.ReviewStep{
			{FromSymbolID: "sym-root", FromCanonicalName: "demo.Root", ToSymbolID: "sym-ambiguous", ToCanonicalName: "demo.Ambiguous"},
		},
		Rejected: []facts.ReviewStep{
			{FromSymbolID: "sym-root", FromCanonicalName: "demo.Root", ToSymbolID: "sym-rejected", ToCanonicalName: "demo.Rejected"},
		},
	}

	svc := NewGraphJSONService()
	data, err := svc.Render(flow)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var result GraphJSON
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, n := range result.Nodes {
		if n.Id == "sym-ambiguous" || n.Id == "sym-rejected" {
			t.Errorf("rejected/ambiguous node should not appear, got: %+v", n)
		}
	}

	for _, e := range result.Edges {
		if e.To == "sym-ambiguous" || e.To == "sym-rejected" {
			t.Errorf("rejected/ambiguous edge should not appear, got: %+v", e)
		}
	}
}
