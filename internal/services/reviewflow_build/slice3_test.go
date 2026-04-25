package reviewflow_build

import (
	"strings"
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/reviewflow_policy"
)

func TestBuildWithOptions_ServicePackHTTPEntryAbstraction(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "boundary_config",
		CanonicalName: "GET /v1/camera/config/all",
		RootType:      entrypoint.RootHTTP,
		Framework:     "gin",
		Method:        "GET",
		Path:          "/v1/camera/config/all",
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "boundary_config", Kind: graph.NodeEndpoint, CanonicalName: "GET /v1/camera/config/all"},
			{ID: "handler_config", Kind: graph.NodeSymbol, CanonicalName: "camera.GetConfig"},
		},
	}
	chain := reduced.Chain{
		RootNodeID: "boundary_config",
		Edges: []reduced.Edge{
			{FromID: "boundary_config", ToID: "handler_config", Label: "dispatch request", OrderIndex: 0},
		},
	}
	audit := flow_stitch.SemanticAuditRoot{
		RootNodeID:    "boundary_config",
		RootCanonical: "GET /v1/camera/config/all",
		HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
			NodeID:        "handler_config",
			CanonicalName: "camera.GetConfig",
		},
	}

	build, err := service.BuildWithOptions(snapshot, root, chain, audit, BuildOptions{
		EntryMode: EntryModeServicePackHTTP,
	})
	if err != nil {
		t.Fatalf("build with service-pack entry mode: %v", err)
	}

	selected := build.Selected
	if selected.ID == "" {
		t.Fatalf("expected selected candidate, got %+v", build)
	}
	if !hasParticipantWithLabel(selected, "Client") {
		t.Fatalf("expected Client participant, got %+v", selected.Participants)
	}
	if !hasParticipantWithLabel(selected, "Gin") {
		t.Fatalf("expected Gin boundary participant, got %+v", selected.Participants)
	}
	if !hasParticipantWithLabel(selected, "Handler") {
		t.Fatalf("expected Handler participant, got %+v", selected.Participants)
	}
	for _, participant := range selected.Participants {
		if isRouteLikeLabel(participant.Label) {
			t.Fatalf("route endpoint should not be visible as participant: %+v", participant)
		}
	}

	clientID := participantIDByLabel(selected, "Client")
	frameworkID := participantIDByLabel(selected, "Gin")
	handlerID := participantIDByLabel(selected, "Handler")
	if clientID == "" || frameworkID == "" || handlerID == "" {
		t.Fatalf("missing expected participant ids: client=%q framework=%q handler=%q", clientID, frameworkID, handlerID)
	}

	if !hasStageMessage(selected, stageBoundaryEntry, clientID, frameworkID, "GET /v1/camera/config/all") {
		t.Fatalf("expected entry request message Client->Gin in boundary entry stage, got %+v", selected.Stages)
	}
	if !hasStageMessage(selected, stageBoundaryEntry, frameworkID, handlerID, "invoke handler") {
		t.Fatalf("expected handler dispatch message Gin->Handler in boundary entry stage, got %+v", selected.Stages)
	}
}

func TestFrameworkBoundaryLabelMapping(t *testing.T) {
	cases := []struct {
		framework string
		want      string
	}{
		{framework: "gin", want: "Gin"},
		{framework: "net/http", want: "net/http"},
		{framework: "grpc-gateway", want: "Gateway Proxy"},
		{framework: "fasthttp", want: "fasthttp"},
		{framework: "", want: "HTTP Router"},
	}

	for _, tc := range cases {
		got := frameworkBoundaryLabel(tc.framework)
		if got != tc.want {
			t.Fatalf("frameworkBoundaryLabel(%q) = %q, want %q", tc.framework, got, tc.want)
		}
	}
}

func TestScoreCandidatesWithPolicy_FamilyInfluencesSelection(t *testing.T) {
	compact := reviewflow.Flow{
		ID:            "compact_candidate",
		RootNodeID:    "boundary_predict",
		CanonicalName: "POST /predict",
		Participants: []reviewflow.Participant{
			{ID: "client", Kind: "client", Label: "Client"},
			{ID: "framework", Kind: "boundary", Label: "Gin"},
			{ID: "handler", Kind: "handler", Label: "Handler"},
		},
		Stages: []reviewflow.Stage{
			{
				Kind: stageBoundaryEntry,
				Messages: []reviewflow.Message{
					{FromParticipantID: "client", ToParticipantID: "framework", Label: "POST /predict", Kind: reviewflow.MessageSync},
					{FromParticipantID: "framework", ToParticipantID: "handler", Label: "invoke handler", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: stageResponse,
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "framework", Label: "return response", Kind: reviewflow.MessageReturn},
				},
			},
		},
		SourceRootType: string(entrypoint.RootHTTP),
		Metadata: reviewflow.Metadata{
			CandidateKind: string(CandidateCompactReview),
			Signature:     "compact_signature",
			RootFramework: "gin",
		},
	}
	rich := reviewflow.Flow{
		ID:            "rich_candidate",
		RootNodeID:    "boundary_predict",
		CanonicalName: "POST /predict",
		Participants: []reviewflow.Participant{
			{ID: "client", Kind: "client", Label: "Client"},
			{ID: "framework", Kind: "boundary", Label: "Gin"},
			{ID: "handler", Kind: "handler", Label: "Handler"},
			{ID: "processor", Kind: "processor", Label: "Detector Processor"},
			{ID: "worker", Kind: "async_sink", Label: "Async Worker"},
		},
		Stages: []reviewflow.Stage{
			{
				Kind: stageBoundaryEntry,
				Messages: []reviewflow.Message{
					{FromParticipantID: "client", ToParticipantID: "framework", Label: "POST /predict", Kind: reviewflow.MessageSync},
					{FromParticipantID: "framework", ToParticipantID: "handler", Label: "invoke handler", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: stageBusinessCore,
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "processor", Label: "process request", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: stagePostProcessing,
				Messages: []reviewflow.Message{
					{FromParticipantID: "processor", ToParticipantID: "handler", Label: "normalize output", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: stageDeferredAsync,
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "worker", Label: "spawn worker", Kind: reviewflow.MessageAsync},
				},
			},
			{
				Kind: stageResponse,
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "framework", Label: "return response", Kind: reviewflow.MessageReturn},
				},
			},
		},
		Blocks: []reviewflow.Block{
			{
				ID:    "branch_block",
				Kind:  reviewflow.BlockAlt,
				Label: "conditional path",
				Sections: []reviewflow.BlockSection{
					{
						Label: "fallback",
						Messages: []reviewflow.Message{
							{FromParticipantID: "handler", ToParticipantID: "processor", Label: "fallback path", Kind: reviewflow.MessageSync},
						},
					},
				},
			},
		},
		SourceRootType: string(entrypoint.RootHTTP),
		Metadata: reviewflow.Metadata{
			CandidateKind: string(CandidateFaithful),
			Signature:     "rich_signature",
			RootFramework: "gin",
		},
	}

	simplePolicy := reviewflow_policy.New().Resolve(reviewflow_policy.ResolveInput{
		ExpectedFamily: reviewflow_policy.FamilySimpleQuery,
	}).Policy
	detectorPolicy := reviewflow_policy.New().Resolve(reviewflow_policy.ResolveInput{
		ExpectedFamily: reviewflow_policy.FamilyDetectorPipeline,
	}).Policy

	simpleBest := selectBestScore(scoreCandidatesWithPolicy([]reviewflow.Flow{compact, rich}, &simplePolicy))
	if simpleBest.FlowID != compact.ID {
		t.Fatalf("expected simple_query policy to prefer compact candidate, got %+v", simpleBest)
	}

	detectorBest := selectBestScore(scoreCandidatesWithPolicy([]reviewflow.Flow{compact, rich}, &detectorPolicy))
	if detectorBest.FlowID != rich.ID {
		t.Fatalf("expected detector_pipeline policy to prefer richer candidate, got %+v", detectorBest)
	}
}

func hasParticipantWithLabel(flow reviewflow.Flow, label string) bool {
	return participantIDByLabel(flow, label) != ""
}

func participantIDByLabel(flow reviewflow.Flow, label string) string {
	for _, participant := range flow.Participants {
		if participant.Label == label {
			return participant.ID
		}
	}
	return ""
}

func hasStageMessage(flow reviewflow.Flow, stageKind, fromID, toID, label string) bool {
	for _, stage := range flow.Stages {
		if stage.Kind != stageKind {
			continue
		}
		for _, message := range stage.Messages {
			if message.FromParticipantID == fromID && message.ToParticipantID == toID && message.Label == label {
				return true
			}
		}
	}
	return false
}

func isRouteLikeLabel(label string) bool {
	parts := strings.SplitN(strings.TrimSpace(label), " ", 2)
	return len(parts) == 2 && isUpperToken(parts[0]) && strings.HasPrefix(parts[1], "/")
}

func isUpperToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
