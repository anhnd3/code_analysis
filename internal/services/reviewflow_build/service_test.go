package reviewflow_build

import (
	"reflect"
	"strings"
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/mermaid_emit"
	"analysis-module/internal/services/reviewflow_policy"
	"analysis-module/internal/services/sequence_model_build"
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
	if first.Selected.Metadata.CandidateKind != second.Selected.Metadata.CandidateKind {
		t.Fatalf("expected deterministic selected candidate kind, got %q vs %q", first.Selected.Metadata.CandidateKind, second.Selected.Metadata.CandidateKind)
	}
	if err := ValidateSelectedBuildResult(first); err != nil {
		t.Fatalf("expected selected build result to validate, got %v", err)
	}
	if err := ValidateSelectedBuildResult(second); err != nil {
		t.Fatalf("expected repeated selected build result to validate, got %v", err)
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

	firstParticipants := participantLabels(first.Selected)
	secondParticipants := participantLabels(second.Selected)
	if !reflect.DeepEqual(firstParticipants, secondParticipants) {
		t.Fatalf("expected deterministic participant labels, got %v vs %v", firstParticipants, secondParticipants)
	}
	firstStages := stageLabels(first.Selected)
	secondStages := stageLabels(second.Selected)
	if !reflect.DeepEqual(firstStages, secondStages) {
		t.Fatalf("expected deterministic stage labels, got %v vs %v", firstStages, secondStages)
	}

	sequenceSvc := sequence_model_build.New()
	mermaidSvc := mermaid_emit.New()
	firstDiagram, err := sequenceSvc.BuildFromReviewFlow(first.Selected, sequence_model_build.Options{Title: "Review"})
	if err != nil {
		t.Fatalf("build first review diagram: %v", err)
	}
	secondDiagram, err := sequenceSvc.BuildFromReviewFlow(second.Selected, sequence_model_build.Options{Title: "Review"})
	if err != nil {
		t.Fatalf("build second review diagram: %v", err)
	}
	firstMermaid, err := mermaidSvc.Emit(firstDiagram)
	if err != nil {
		t.Fatalf("emit first review mermaid: %v", err)
	}
	secondMermaid, err := mermaidSvc.Emit(secondDiagram)
	if err != nil {
		t.Fatalf("emit second review mermaid: %v", err)
	}
	if firstMermaid != secondMermaid {
		t.Fatalf("expected deterministic Mermaid output, got:\n%s\n---\n%s", firstMermaid, secondMermaid)
	}
	assertNoVisibleReviewLeaks(t, first.Selected, firstMermaid)
}

func TestBuildWithOptions_DefaultMatchesBuild(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "boundary_health",
		CanonicalName: "GET /health",
		RootType:      entrypoint.RootHTTP,
		Framework:     "gin",
		Method:        "GET",
		Path:          "/health",
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "boundary_health", Kind: graph.NodeEndpoint, CanonicalName: "GET /health"},
			{ID: "handler_health", Kind: graph.NodeSymbol, CanonicalName: "api.Health", Properties: map[string]string{"name": "Health", "kind": "function"}},
		},
	}
	chain := reduced.Chain{
		RootNodeID: "boundary_health",
		Edges: []reduced.Edge{
			{FromID: "boundary_health", ToID: "handler_health", Label: "dispatch request", OrderIndex: 0},
		},
	}
	audit := flow_stitch.SemanticAuditRoot{
		RootNodeID:    "boundary_health",
		RootCanonical: "GET /health",
		HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
			NodeID:        "handler_health",
			CanonicalName: "api.Health",
		},
	}

	defaultBuild, err := service.Build(snapshot, root, chain, audit)
	if err != nil {
		t.Fatalf("build default: %v", err)
	}
	optionBuild, err := service.BuildWithOptions(snapshot, root, chain, audit, BuildOptions{})
	if err != nil {
		t.Fatalf("build with options default: %v", err)
	}

	if defaultBuild.SelectedID != optionBuild.SelectedID || defaultBuild.Signature != optionBuild.Signature {
		t.Fatalf("expected Build and BuildWithOptions default to match, got %+v vs %+v", defaultBuild, optionBuild)
	}

	_, err = service.BuildWithPolicy(snapshot, root, chain, audit, reviewflow_policy.Policy{
		Family:                  reviewflow_policy.FamilySimpleQuery,
		PreferredCandidateKinds: []string{"faithful", "compact_review", "async_summarized"},
	})
	if err != nil {
		t.Fatalf("build with policy: %v", err)
	}
}

func TestBuild_StableAcrossSemanticAuditEdgeIDChanges(t *testing.T) {
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
			{
				ID:            "session_load",
				Kind:          graph.NodeSymbol,
				CanonicalName: "session.GetSessionByToken",
				Properties: map[string]string{
					"name": "GetSessionByToken",
					"kind": "service",
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

	firstAudit := flow_stitch.SemanticAuditRoot{
		RootNodeID:    "boundary_process",
		RootCanonical: "POST /process",
		RegistersBoundaryEdge: &flow_stitch.SemanticAuditEdgeRef{
			EdgeID:          "edge_register_first",
			Kind:            "REGISTERS_BOUNDARY",
			FromNodeID:      "boundary_process",
			ToNodeID:        "handler_inline",
			Label:           "$inline_handler_0",
			ResolutionBasis: "symbol_hint",
		},
		ReturnsHandlerEdge: &flow_stitch.SemanticAuditEdgeRef{
			EdgeID:          "edge_return_first",
			Kind:            "RETURNS_HANDLER",
			FromNodeID:      "boundary_process",
			ToNodeID:        "handler_inline",
			Label:           "$inline_handler_0",
			ResolutionBasis: "symbol_hint",
		},
		HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
			NodeID:        "handler_inline",
			CanonicalName: "main.main.$inline_handler_0",
			ShortName:     "$inline_handler_0",
		},
		FirstBusinessCalls: []flow_stitch.SemanticAuditEdgeRef{
			{
				EdgeID:          "edge_business_z",
				Kind:            "CALLS",
				FromNodeID:      "handler_inline",
				ToNodeID:        "session_load",
				Label:           "Get Session By Token",
				ResolutionBasis: "package_method_hint",
			},
			{
				EdgeID:          "edge_business_a",
				Kind:            "CALLS",
				FromNodeID:      "handler_inline",
				ToNodeID:        "repo_detect",
				Label:           "DetectQR",
				ResolutionBasis: "package_method_hint",
			},
		},
	}
	secondAudit := firstAudit
	secondAudit.RegistersBoundaryEdge = &flow_stitch.SemanticAuditEdgeRef{
		EdgeID:          "edge_register_second",
		Kind:            "REGISTERS_BOUNDARY",
		FromNodeID:      "boundary_process",
		ToNodeID:        "handler_inline",
		Label:           "$inline_handler_0",
		ResolutionBasis: "symbol_hint",
	}
	secondAudit.ReturnsHandlerEdge = &flow_stitch.SemanticAuditEdgeRef{
		EdgeID:          "edge_return_second",
		Kind:            "RETURNS_HANDLER",
		FromNodeID:      "boundary_process",
		ToNodeID:        "handler_inline",
		Label:           "$inline_handler_0",
		ResolutionBasis: "symbol_hint",
	}
	secondAudit.FirstBusinessCalls = []flow_stitch.SemanticAuditEdgeRef{
		{
			EdgeID:          "edge_business_2",
			Kind:            "CALLS",
			FromNodeID:      "handler_inline",
			ToNodeID:        "repo_detect",
			Label:           "DetectQR",
			ResolutionBasis: "package_method_hint",
		},
		{
			EdgeID:          "edge_business_1",
			Kind:            "CALLS",
			FromNodeID:      "handler_inline",
			ToNodeID:        "session_load",
			Label:           "Get Session By Token",
			ResolutionBasis: "package_method_hint",
		},
	}

	first, err := service.Build(snapshot, root, chain, firstAudit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Build(snapshot, root, chain, secondAudit)
	if err != nil {
		t.Fatal(err)
	}

	if first.SelectedID != second.SelectedID {
		t.Fatalf("expected selected id to ignore semantic audit edge ids, got %q vs %q", first.SelectedID, second.SelectedID)
	}
	if first.Signature != second.Signature {
		t.Fatalf("expected signature to ignore semantic audit edge ids, got %q vs %q", first.Signature, second.Signature)
	}
	if !reflect.DeepEqual(first.Selected, second.Selected) {
		t.Fatalf("expected selected reviewflow to remain stable when only semantic audit edge ids change")
	}

	sequenceSvc := sequence_model_build.New()
	mermaidSvc := mermaid_emit.New()
	firstDiagram, err := sequenceSvc.BuildFromReviewFlow(first.Selected, sequence_model_build.Options{Title: "Review"})
	if err != nil {
		t.Fatalf("build first review diagram: %v", err)
	}
	secondDiagram, err := sequenceSvc.BuildFromReviewFlow(second.Selected, sequence_model_build.Options{Title: "Review"})
	if err != nil {
		t.Fatalf("build second review diagram: %v", err)
	}
	firstMermaid, err := mermaidSvc.Emit(firstDiagram)
	if err != nil {
		t.Fatalf("emit first review mermaid: %v", err)
	}
	secondMermaid, err := mermaidSvc.Emit(secondDiagram)
	if err != nil {
		t.Fatalf("emit second review mermaid: %v", err)
	}
	if firstMermaid != secondMermaid {
		t.Fatalf("expected Mermaid output to remain stable when only semantic audit edge ids change")
	}
}

func TestValidateSelectedBuildResult_RejectsVisibleArtifacts(t *testing.T) {
	flow := reviewflow.Flow{
		ID:            "reviewflow_invalid",
		RootNodeID:    "boundary_process",
		CanonicalName: "POST /process",
		Participants: []reviewflow.Participant{
			{ID: "boundary", Kind: "boundary", Label: "POST /process"},
			{ID: "handler", Kind: "handler", Label: "$inline_handler_0"},
		},
		Stages: []reviewflow.Stage{
			{
				ID:             "stage_boundary_entry",
				Kind:           stageBoundaryEntry,
				Label:          "Boundary Entry",
				ParticipantIDs: []string{"boundary", "handler"},
				Messages: []reviewflow.Message{
					{ID: "msg_go_func", FromParticipantID: "boundary", ToParticipantID: "handler", Label: "go func() { wg.Done() }", Kind: reviewflow.MessageSync},
				},
			},
		},
		Metadata: reviewflow.Metadata{
			CandidateKind: string(CandidateCompactReview),
			Signature:     "invalid_signature",
			RootFramework: "gin",
		},
	}
	result := BuildResult{
		Selected:   flow,
		Candidates: []reviewflow.Flow{flow},
		SelectedID: flow.ID,
		Signature:  flow.Metadata.Signature,
	}
	err := ValidateSelectedBuildResult(result)
	if err == nil {
		t.Fatal("expected selected build result with visible artifacts to fail validation")
	}
	if !strings.Contains(err.Error(), ErrSelectedOutputLeak.Error()) {
		t.Fatalf("expected selected output leak error, got %v", err)
	}
}

func TestBuild_UsesStructuredAuditForSessionOrderAndGuardBlocks(t *testing.T) {
	service := New()
	root := entrypoint.Root{
		NodeID:        "boundary_detect",
		CanonicalName: "POST /detect",
		RootType:      entrypoint.RootHTTP,
		Framework:     "gin",
		Method:        "POST",
		Path:          "/detect",
	}
	snapshot := graph.GraphSnapshot{
		Nodes: []graph.Node{
			{ID: "boundary_detect", Kind: graph.NodeEndpoint, CanonicalName: "POST /detect"},
			{ID: "handler_detect", Kind: graph.NodeSymbol, CanonicalName: "api.HandleDetect", Properties: map[string]string{"name": "HandleDetect", "kind": "function"}},
			{ID: "session_load", Kind: graph.NodeSymbol, CanonicalName: "session.GetSessionByZlpToken", Properties: map[string]string{"name": "GetSessionByZlpToken", "kind": "service"}},
			{ID: "repo_detect", Kind: graph.NodeSymbol, CanonicalName: "repo.CameraRepo.DetectQR", Properties: map[string]string{"name": "DetectQR", "kind": "repository"}},
			{ID: "post_detect", Kind: graph.NodeSymbol, CanonicalName: "repo.postProcessingQRResults", Properties: map[string]string{"name": "postProcessingQRResults", "kind": "function"}},
		},
	}
	chain := reduced.Chain{
		RootNodeID: "boundary_detect",
		Nodes: []reduced.Node{
			{ID: "boundary_detect", ShortName: "POST /detect", CanonicalName: "POST /detect", Role: reduced.RoleRoot},
			{ID: "handler_detect", ShortName: "HandleDetect", CanonicalName: "api.HandleDetect", Role: reduced.RoleHandler},
			{ID: "repo_detect", ShortName: "DetectQR", CanonicalName: "repo.CameraRepo.DetectQR", Role: reduced.RoleRepository},
		},
		Edges: []reduced.Edge{
			{FromID: "boundary_detect", ToID: "handler_detect", Label: "dispatch request", OrderIndex: 0},
			{FromID: "handler_detect", ToID: "repo_detect", Label: "DetectQR", OrderIndex: 3},
		},
	}
	audit := flow_stitch.SemanticAuditRoot{
		RootNodeID:    "boundary_detect",
		RootCanonical: "POST /detect",
		HandlerTargetNode: &flow_stitch.SemanticAuditNodeRef{
			NodeID:        "handler_detect",
			CanonicalName: "api.HandleDetect",
		},
		LeadingCalls: []flow_stitch.SemanticAuditEdgeRef{
			{EdgeID: "e_session", Kind: "CALLS", FromNodeID: "handler_detect", ToNodeID: "session_load", Label: "GetSessionByZlpToken", OrderIndex: 1},
		},
		BusinessHandoff: []flow_stitch.SemanticAuditEdgeRef{
			{EdgeID: "e_repo", Kind: "CALLS", FromNodeID: "handler_detect", ToNodeID: "repo_detect", Label: "DetectQR", OrderIndex: 3},
		},
		DrilldownCalls: []flow_stitch.SemanticAuditEdgeRef{
			{EdgeID: "e_post", Kind: "CALLS", FromNodeID: "repo_detect", ToNodeID: "post_detect", Label: "postProcessingQRResults", OrderIndex: 0},
		},
		GuardSummaries: []flow_stitch.SemanticAuditGuard{
			{Kind: "session_invalid", StageKind: stageSessionContext, Condition: "session invalid", Outcome: "return unauthorized", OrderIndex: 1},
		},
	}

	build, err := service.Build(snapshot, root, chain, audit)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSelectedBuildResult(build); err != nil {
		t.Fatalf("expected selected build to validate, got %v", err)
	}

	selected := build.Selected
	if !hasStageKind(selected, stageSessionContext) {
		t.Fatalf("expected selected flow to include a session context stage, got %+v", selected.Stages)
	}
	if !hasStageKind(selected, stageBusinessCore) {
		t.Fatalf("expected selected flow to include a business core stage, got %+v", selected.Stages)
	}

	sessionIndex := stageIndex(selected, stageSessionContext)
	businessIndex := stageIndex(selected, stageBusinessCore)
	if sessionIndex < 0 || businessIndex < 0 || sessionIndex >= businessIndex {
		t.Fatalf("expected session stage before business stage, got %+v", selected.Stages)
	}

	sessionStage := selected.Stages[sessionIndex]
	if len(sessionStage.Messages) == 0 || !strings.Contains(strings.ToLower(sessionStage.Messages[0].Label), "session") {
		t.Fatalf("expected session stage to contain recovered session message, got %+v", sessionStage)
	}

	if len(selected.Blocks) == 0 {
		t.Fatalf("expected selected flow to include inferred guard block, got %+v", selected)
	}
	foundGuard := false
	for _, block := range selected.Blocks {
		if block.StageID != stageSessionContext {
			continue
		}
		if strings.Contains(strings.ToLower(block.Label), "session invalid") {
			foundGuard = true
			break
		}
	}
	if !foundGuard {
		t.Fatalf("expected session guard block in selected flow, got %+v", selected.Blocks)
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

func participantLabels(flow reviewflow.Flow) []string {
	labels := make([]string, 0, len(flow.Participants))
	for _, participant := range flow.Participants {
		labels = append(labels, participant.Label)
	}
	return labels
}

func stageLabels(flow reviewflow.Flow) []string {
	labels := make([]string, 0, len(flow.Stages))
	for _, stage := range flow.Stages {
		labels = append(labels, stage.Kind+":"+stage.Label)
	}
	return labels
}

func stageIndex(flow reviewflow.Flow, kind string) int {
	for i, stage := range flow.Stages {
		if stage.Kind == kind {
			return i
		}
	}
	return -1
}

func assertNoVisibleReviewLeaks(t *testing.T, flow reviewflow.Flow, mermaid string) {
	t.Helper()

	for _, participant := range flow.Participants {
		if visibleReviewArtifactLeak(participant.Label) {
			t.Fatalf("unexpected visible artifact in participant %+v", participant)
		}
	}
	for _, stage := range flow.Stages {
		if visibleReviewArtifactLeak(stage.Label) {
			t.Fatalf("unexpected visible artifact in stage %+v", stage)
		}
		for _, message := range stage.Messages {
			if visibleReviewArtifactLeak(message.Label) {
				t.Fatalf("unexpected visible artifact in message %+v", message)
			}
		}
	}
	lowerMermaid := strings.ToLower(mermaid)
	for _, token := range []string{"$closure_", "$inline_", "go func", "func()", "wg.", "defer ", "<-"} {
		if strings.Contains(lowerMermaid, token) {
			t.Fatalf("unexpected raw artifact token %q in Mermaid output:\n%s", token, mermaid)
		}
	}
}
