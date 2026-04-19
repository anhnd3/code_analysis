package reviewflow_build

import (
	"strings"
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
)

func TestBuild_ProducesDeterministicReviewflowAndSuppressesInlineArtifacts(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "boundary_process",
		CanonicalName: "POST /process",
		RootType:      entrypoint.RootHTTP,
		Framework:     "gin",
		Method:        "POST",
		Path:          "/process",
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "boundary_process", Kind: graph.NodeEndpoint, CanonicalName: "POST /process"},
			{
				ID:            "handler_inline",
				Kind:          graph.NodeSymbol,
				CanonicalName: "main.main.$inline_handler_0",
				Properties: map[string]string{
					"name":           "$inline_handler_0",
					"synthetic":      "true",
					"synthetic_kind": "inline_handler",
				},
			},
			{
				ID:            "repo_detect",
				Kind:          graph.NodeSymbol,
				CanonicalName: "repo.DetectQR",
				Properties: map[string]string{
					"name": "DetectQR",
					"kind": "repository",
				},
			},
		},
		Edges: []graph.Edge{
			{Kind: graph.EdgeRegistersBoundary, To: "handler_inline"},
		},
	}
	chain := reduced.Chain{
		RootNodeID: "boundary_process",
		Nodes: []reduced.Node{
			{ID: "boundary_process", ShortName: "POST /process", CanonicalName: "POST /process", Role: reduced.RoleRoot},
			{ID: "handler_inline", ShortName: "$inline_handler_0", CanonicalName: "main.main.$inline_handler_0", Role: reduced.RoleBoundary},
		},
		Edges: []reduced.Edge{
			{FromID: "boundary_process", ToID: "handler_inline", Label: "$inline_handler_0", OrderIndex: 0},
		},
	}
	audit := flow_stitch.SemanticAuditRoot{
		RootNodeID:    "boundary_process",
		RootCanonical: "POST /process",
		HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
			NodeID:        "handler_inline",
			CanonicalName: "main.main.$inline_handler_0",
			ShortName:     "$inline_handler_0",
		},
		FirstBusinessCalls: []flow_stitch.SemanticAuditEdgeRef{
			{
				EdgeID:     "edge_business",
				FromNodeID: "handler_inline",
				ToNodeID:   "repo_detect",
				Label:      "DetectQR",
			},
		},
	}

	first, err := service.Build(snapshot, root, chain, audit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Build(snapshot, root, chain, audit)
	if err != nil {
		t.Fatal(err)
	}

	if len(first.Candidates) != 3 {
		t.Fatalf("expected exactly three deterministic candidates, got %d", len(first.Candidates))
	}
	if first.SelectedID == "" || first.Signature == "" {
		t.Fatalf("expected selected id and signature, got %+v", first)
	}
	if first.SelectedID != second.SelectedID {
		t.Fatalf("expected deterministic selected id, got %q vs %q", first.SelectedID, second.SelectedID)
	}
	if first.Signature != second.Signature {
		t.Fatalf("expected deterministic signature, got %q vs %q", first.Signature, second.Signature)
	}

	for _, participant := range first.Selected.Participants {
		if strings.Contains(participant.Label, "$inline") {
			t.Fatalf("expected inline handler artifact to be suppressed, got participant %+v", participant)
		}
	}
	if !hasParticipantLabel(first.Selected, "Handler") {
		t.Fatalf("expected stable Handler participant in selected flow, got %+v", first.Selected.Participants)
	}
	if !hasStageKind(first.Selected, stageBusinessCore) {
		t.Fatalf("expected business core stage in selected flow, got %+v", first.Selected.Stages)
	}
}

func hasParticipantLabel(flow reviewflow.Flow, want string) bool {
	for _, participant := range flow.Participants {
		if participant.Label == want {
			return true
		}
	}
	return false
}

func hasStageKind(flow reviewflow.Flow, kind string) bool {
	for _, stage := range flow.Stages {
		if stage.Kind == kind {
			return true
		}
	}
	return false
}
