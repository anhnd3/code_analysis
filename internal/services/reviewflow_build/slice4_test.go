package reviewflow_build

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/reviewflow_expand"
	"analysis-module/internal/services/reviewflow_policy"
)

func TestBuildWithOptions_ExpansionEnabledAddsEvidenceBackedDetail(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "boundary_detect",
		CanonicalName: "POST /v1/camera/detect-qr",
		RootType:      entrypoint.RootHTTP,
		Framework:     "gin",
		Method:        "POST",
		Path:          "/v1/camera/detect-qr",
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "boundary_detect", Kind: graph.NodeEndpoint, CanonicalName: "POST /v1/camera/detect-qr"},
			{ID: "handler_detect", Kind: graph.NodeSymbol, CanonicalName: "camera.Handler", Properties: map[string]string{"name": "HandleDetectQR", "kind": "route_handler"}},
			{ID: "repo_detect", Kind: graph.NodeSymbol, CanonicalName: "cameraRepo.DetectQR", Properties: map[string]string{"name": "DetectQR", "kind": "repository"}},
			{ID: "post_process", Kind: graph.NodeSymbol, CanonicalName: "processor.postProcessQrResponse", Properties: map[string]string{"name": "postProcessQrResponse", "kind": "processor"}},
		},
		Edges: []graph.Edge{
			{ID: "e_call_repo", Kind: graph.EdgeCalls, From: "handler_detect", To: "repo_detect", Properties: map[string]string{"order_index": "1"}},
			{ID: "e_call_post", Kind: graph.EdgeCalls, From: "repo_detect", To: "post_process", Properties: map[string]string{"order_index": "2"}},
		},
	}
	chain := reduced.Chain{
		RootNodeID: "boundary_detect",
		Edges: []reduced.Edge{
			{FromID: "boundary_detect", ToID: "handler_detect", Label: "dispatch request", OrderIndex: 0},
		},
	}
	audit := flow_stitch.SemanticAuditRoot{
		RootNodeID:    "boundary_detect",
		RootCanonical: "POST /v1/camera/detect-qr",
		HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
			NodeID:        "handler_detect",
			CanonicalName: "camera.Handler",
		},
		FirstBusinessCalls: []flow_stitch.SemanticAuditEdgeRef{{
			EdgeID:     "e_call_repo",
			Kind:       string(graph.EdgeCalls),
			FromNodeID: "handler_detect",
			ToNodeID:   "repo_detect",
			Label:      "DetectQR",
			OrderIndex: 1,
		}},
	}
	policy := reviewflow_policy.Policy{
		Family:                    reviewflow_policy.FamilyDetectorPipeline,
		MinBusinessExpansionDepth: 1,
		MaxBusinessExpansionDepth: 2,
		AddHTTPEntryParticipants:  true,
	}

	withoutExpansion, err := service.BuildWithOptions(snapshot, root, chain, audit, BuildOptions{
		Policy: &policy,
	})
	if err != nil {
		t.Fatalf("build without expansion: %v", err)
	}

	withExpansion, err := service.BuildWithOptions(snapshot, root, chain, audit, BuildOptions{
		Policy:           &policy,
		EntryMode:        EntryModeServicePackHTTP,
		ExpansionEnabled: true,
	})
	if err != nil {
		t.Fatalf("build with expansion: %v", err)
	}

	if withExpansion.ExpansionMetadata == nil {
		t.Fatalf("expected expansion metadata when expansion is enabled, got %+v", withExpansion)
	}
	if withExpansion.ExpansionMetadata.AddedMessages == 0 {
		t.Fatalf("expected evidence-backed expanded messages, got %+v", withExpansion.ExpansionMetadata)
	}
	if reviewFlowMessageCount(withExpansion.Selected) <= reviewFlowMessageCount(withoutExpansion.Selected) {
		t.Fatalf("expected expanded flow to contain more detail, before=%d after=%d", reviewFlowMessageCount(withoutExpansion.Selected), reviewFlowMessageCount(withExpansion.Selected))
	}
	if !hasSelectedMessageClass(withExpansion.Selected, reviewflow_expand.ClassPostProcessing) {
		t.Fatalf("expected post-processing semantic class from expansion, got %+v", withExpansion.Selected.Stages)
	}
}

func hasSelectedMessageClass(flow reviewflow.Flow, class string) bool {
	for _, stage := range flow.Stages {
		for _, message := range stage.Messages {
			if message.Class == class {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		for _, section := range block.Sections {
			for _, message := range section.Messages {
				if message.Class == class {
					return true
				}
			}
		}
	}
	return false
}

func reviewFlowMessageCount(flow reviewflow.Flow) int {
	count := 0
	for _, stage := range flow.Stages {
		count += len(stage.Messages)
	}
	for _, block := range flow.Blocks {
		for _, section := range block.Sections {
			count += len(section.Messages)
		}
	}
	return count
}
