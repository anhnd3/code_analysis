package reviewflow_expand

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/reviewflow_policy"
)

func TestExpand_BoundedDepthAndNoiseFiltering(t *testing.T) {
	svc := New()
	input := Input{
		Snapshot: graph.GraphSnapshot{
			Nodes: []graph.Node{
				{ID: "handler", Kind: graph.NodeSymbol, CanonicalName: "camera.Handler", Properties: map[string]string{"name": "HandleDetectQR", "kind": "route_handler"}},
				{ID: "repo", Kind: graph.NodeSymbol, CanonicalName: "cameraRepo.DetectQR", Properties: map[string]string{"name": "DetectQR", "kind": "repository"}},
				{ID: "post", Kind: graph.NodeSymbol, CanonicalName: "processor.postProcessQrResponse", Properties: map[string]string{"name": "postProcessQrResponse", "kind": "processor"}},
				{ID: "logger", Kind: graph.NodeSymbol, CanonicalName: "logging.Logger.Trace", Properties: map[string]string{"name": "Trace", "kind": "function"}},
			},
			Edges: []graph.Edge{
				{ID: "e1", Kind: graph.EdgeCalls, From: "handler", To: "repo", Properties: map[string]string{"order_index": "1"}},
				{ID: "e2", Kind: graph.EdgeCalls, From: "repo", To: "post", Properties: map[string]string{"order_index": "2"}},
				{ID: "e3", Kind: graph.EdgeCalls, From: "handler", To: "logger", Properties: map[string]string{"order_index": "3"}},
			},
		},
		Root: entrypoint.Root{NodeID: "handler", RootType: entrypoint.RootHTTP, CanonicalName: "POST /v1/camera/detect-qr"},
		Reduced: reduced.Chain{
			RootNodeID: "handler",
		},
		Audit: flow_stitch.SemanticAuditRoot{
			RootNodeID: "handler",
			HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
				NodeID:        "handler",
				CanonicalName: "camera.Handler",
			},
			FirstBusinessCalls: []flow_stitch.SemanticAuditEdgeRef{{
				EdgeID:     "e1",
				Kind:       string(graph.EdgeCalls),
				FromNodeID: "handler",
				ToNodeID:   "repo",
				Label:      "DetectQR",
				OrderIndex: 1,
			}},
		},
		Selected: baseSelectedFlow(),
		Policy: reviewflow_policy.Policy{
			Family:                    reviewflow_policy.FamilyDetectorPipeline,
			MinBusinessExpansionDepth: 1,
			MaxBusinessExpansionDepth: 2,
		},
	}

	result, err := svc.Expand(input)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if result.Metadata.MaxDepth != 2 {
		t.Fatalf("expected max depth 2, got %+v", result.Metadata)
	}
	if !hasMessageLabel(result.Flow, "post Process QR Response") {
		t.Fatalf("expected depth-2 post-processing evidence to be expanded, got %+v", result.Flow.Stages)
	}
	if hasMessageLabel(result.Flow, "Trace") {
		t.Fatalf("expected demoted logging noise to be filtered, got %+v", result.Flow.Stages)
	}
	if result.Metadata.DemotedNoise == 0 {
		t.Fatalf("expected noise demotion metadata to be reported, got %+v", result.Metadata)
	}
}

func TestExpand_ClassifiesBranchAndAsyncEvidence(t *testing.T) {
	svc := New()
	input := Input{
		Snapshot: graph.GraphSnapshot{
			Nodes: []graph.Node{
				{ID: "handler", Kind: graph.NodeSymbol, CanonicalName: "camera.Handler", Properties: map[string]string{"name": "HandleDetectQR", "kind": "route_handler"}},
				{ID: "validator", Kind: graph.NodeSymbol, CanonicalName: "validation.ValidateRequest", Properties: map[string]string{"name": "ValidateRequest", "kind": "function"}},
				{ID: "worker", Kind: graph.NodeSymbol, CanonicalName: "worker.detectQRWorker", Properties: map[string]string{"name": "detectQRWorker", "kind": "processor"}},
				{ID: "cleanup", Kind: graph.NodeSymbol, CanonicalName: "push.deferZDSPush", Properties: map[string]string{"name": "deferZDSPush", "kind": "function"}},
				{ID: "response", Kind: graph.NodeSymbol, CanonicalName: "http.response", Properties: map[string]string{"name": "Respond", "kind": "function"}},
			},
			Edges: []graph.Edge{
				{ID: "c1", Kind: graph.EdgeCalls, From: "handler", To: "validator", Properties: map[string]string{"order_index": "1"}},
				{ID: "s1", Kind: graph.EdgeSpawns, From: "handler", To: "worker", Properties: map[string]string{"order_index": "2"}},
				{ID: "d1", Kind: graph.EdgeDefers, From: "handler", To: "cleanup", Properties: map[string]string{"order_index": "3"}},
			},
		},
		Root: entrypoint.Root{NodeID: "handler", RootType: entrypoint.RootHTTP, CanonicalName: "POST /v2/camera/detect-qr"},
		Reduced: reduced.Chain{
			RootNodeID: "handler",
			Blocks: []reduced.Block{{
				Kind: reduced.BlockAlt,
				Branches: []reduced.Branch{{
					Condition: "validation failed",
					Edges: []reduced.Edge{{
						FromID:     "handler",
						ToID:       "response",
						Label:      "validation failed",
						OrderIndex: 4,
					}},
				}},
			}},
		},
		Audit: flow_stitch.SemanticAuditRoot{
			RootNodeID: "handler",
			HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
				NodeID:        "handler",
				CanonicalName: "camera.Handler",
			},
		},
		Selected: baseSelectedFlow(),
		Policy: reviewflow_policy.Policy{
			Family:                    reviewflow_policy.FamilyDetectorPipeline,
			MinBusinessExpansionDepth: 1,
			MaxBusinessExpansionDepth: 2,
		},
	}

	result, err := svc.Expand(input)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !hasMessageClass(result.Flow, ClassValidation) {
		t.Fatalf("expected validation class in expanded messages, got %+v", result.Flow.Stages)
	}
	if !hasMessageClass(result.Flow, ClassAsyncWorker) {
		t.Fatalf("expected async worker class in expanded messages, got %+v", result.Flow.Stages)
	}
	if !hasMessageClass(result.Flow, ClassDeferredSideEffect) {
		t.Fatalf("expected deferred side-effect class in expanded messages, got %+v", result.Flow.Stages)
	}
	if !hasBlockClass(result.Flow, ClassBranchValidation) {
		t.Fatalf("expected validation branch block class, got %+v", result.Flow.Blocks)
	}
}

func TestExpand_DoesNotInventMessagesWithoutEvidence(t *testing.T) {
	svc := New()
	selected := baseSelectedFlow()
	before := messageCount(selected)

	result, err := svc.Expand(Input{
		Snapshot: graph.GraphSnapshot{},
		Root: entrypoint.Root{
			NodeID:        "handler",
			RootType:      entrypoint.RootHTTP,
			CanonicalName: "POST /scan360/v1/predict",
		},
		Reduced: reduced.Chain{RootNodeID: "handler"},
		Audit: flow_stitch.SemanticAuditRoot{
			RootNodeID: "handler",
			HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
				NodeID:        "handler",
				CanonicalName: "predict.Handler",
			},
		},
		Selected: selected,
		Policy: reviewflow_policy.Policy{
			Family:                    reviewflow_policy.FamilyScanPipeline,
			MinBusinessExpansionDepth: 1,
			MaxBusinessExpansionDepth: 3,
		},
	})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	after := messageCount(result.Flow)
	if before != after {
		t.Fatalf("expected no invented messages without evidence, before=%d after=%d", before, after)
	}
	if result.Metadata.AddedMessages != 0 {
		t.Fatalf("expected zero added messages without evidence, got %+v", result.Metadata)
	}
}

func baseSelectedFlow() reviewflow.Flow {
	return reviewflow.Flow{
		ID:             "selected",
		RootNodeID:     "handler",
		CanonicalName:  "POST /root",
		SourceRootType: string(entrypoint.RootHTTP),
		Participants: []reviewflow.Participant{
			{ID: "boundary", Kind: "boundary", Label: "Gin"},
			{ID: "handler", Kind: "handler", Label: "Handler", SourceNodeIDs: []string{"handler"}},
		},
		Stages: []reviewflow.Stage{{
			ID:             "boundary_entry",
			Kind:           "boundary_entry",
			Label:          "Boundary Entry",
			ParticipantIDs: []string{"boundary", "handler"},
			Messages: []reviewflow.Message{{
				ID:                "entry",
				FromParticipantID: "boundary",
				ToParticipantID:   "handler",
				Label:             "invoke handler",
				Kind:              reviewflow.MessageSync,
			}},
		}},
	}
}

func hasMessageLabel(flow reviewflow.Flow, label string) bool {
	for _, stage := range flow.Stages {
		for _, message := range stage.Messages {
			if message.Label == label {
				return true
			}
		}
	}
	return false
}

func hasMessageClass(flow reviewflow.Flow, class string) bool {
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

func hasBlockClass(flow reviewflow.Flow, class string) bool {
	for _, block := range flow.Blocks {
		if block.Class == class {
			return true
		}
	}
	return false
}

func messageCount(flow reviewflow.Flow) int {
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
