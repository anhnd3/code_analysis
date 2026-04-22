package export_mermaid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/sequence"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/mermaid_emit"
	"analysis-module/internal/services/reviewflow_build"
	"analysis-module/internal/services/sequence_model_build"
)

func TestEnsureNonEmptyStageGuards(t *testing.T) {
	t.Run("roots", func(t *testing.T) {
		if err := ensureNonEmptyRoots(entrypoint.Result{}, RootFilterHTTP); err == nil {
			t.Fatal("expected empty-root guard to fail")
		}
	})

	t.Run("chains", func(t *testing.T) {
		if err := ensureNonEmptyChains(flow.Bundle{}); err == nil {
			t.Fatal("expected empty-chain guard to fail")
		}
	})

	t.Run("reduced", func(t *testing.T) {
		if err := ensureNonEmptyReducedChain(reduced.Chain{}); err == nil {
			t.Fatal("expected empty-reduced-chain guard to fail")
		}
	})

	t.Run("sequence", func(t *testing.T) {
		if err := ensureNonEmptySequence(sequence.Diagram{}); err == nil {
			t.Fatal("expected empty-sequence guard to fail")
		}
	})
}

func TestFilterRootsHonorsSelectorWithinType(t *testing.T) {
	full := entrypoint.Result{
		Roots: []entrypoint.Root{
			{NodeID: "root_health", CanonicalName: "GET /health", RootType: entrypoint.RootHTTP},
			{NodeID: "root_info", CanonicalName: "GET /info", RootType: entrypoint.RootHTTP},
			{NodeID: "bootstrap_start", CanonicalName: "cmd.Start", RootType: entrypoint.RootBootstrap},
		},
	}

	filtered := filterRoots(full, Request{
		RootType:     RootFilterHTTP,
		RootSelector: "GET /info",
	})
	if len(filtered.Roots) != 1 {
		t.Fatalf("expected one filtered root, got %+v", filtered.Roots)
	}
	if filtered.Roots[0].CanonicalName != "GET /info" {
		t.Fatalf("expected GET /info, got %+v", filtered.Roots[0])
	}

	filteredByNode := filterRoots(full, Request{
		RootType:     RootFilterHTTP,
		RootSelector: "root_health",
	})
	if len(filteredByNode.Roots) != 1 || filteredByNode.Roots[0].NodeID != "root_health" {
		t.Fatalf("expected node-id selector to narrow to root_health, got %+v", filteredByNode.Roots)
	}
}

func TestRenderModeForRoot_DefaultsHTTPToReviewWhenMetadataPresent(t *testing.T) {
	w := Workflow{}
	mode := w.renderModeForRoot(Request{}, entrypoint.Root{
		NodeID:    "root_health",
		RootType:  entrypoint.RootHTTP,
		Framework: "gin",
		Method:    "GET",
		Path:      "/health",
	})
	if mode != RenderModeReview {
		t.Fatalf("expected HTTP root with metadata to default to review, got %s", mode)
	}
}

func TestRenderModeForRoot_HonorsExplicitReducedDebug(t *testing.T) {
	w := Workflow{}
	mode := w.renderModeForRoot(Request{RenderMode: RenderModeReducedDebug}, entrypoint.Root{
		NodeID:   "root_health",
		RootType: entrypoint.RootHTTP,
	})
	if mode != RenderModeReducedDebug {
		t.Fatalf("expected explicit reduced_debug mode, got %s", mode)
	}
}

func TestRenderRootStrictReviewFailsOnEmptySelection(t *testing.T) {
	w := testWorkflowWithReviewBuilder(stubReviewFlowBuilder{
		result: reviewflow_build.BuildResult{
			Candidates: []reviewflow.Flow{validSelectedReviewFlow()},
		},
	})

	rendered, decision, err := w.renderRoot(
		Request{RenderMode: RenderModeReview, ReviewStrict: true},
		graph.GraphSnapshot{},
		testHTTPRoot(),
		testReducedChain(),
		flow_stitch.SemanticAuditRoot{},
		sequence_model_build.Options{Title: "Review"},
	)
	if err == nil {
		t.Fatal("expected strict review mode to fail on empty selection")
	}
	if !strings.Contains(err.Error(), string(ReviewFallbackReasonNoSelectedCandidate)) {
		t.Fatalf("expected no_selected_candidate error, got %v", err)
	}
	if rendered.reviewFlowBuild == nil {
		t.Fatal("expected review build artifact to be kept for debug output")
	}
	if decision.UsedRenderer != "" {
		t.Fatalf("expected no final renderer on strict failure, got %q", decision.UsedRenderer)
	}
	if !decision.ReviewAttempted || decision.ReviewSelected || decision.FallbackUsed {
		t.Fatalf("unexpected strict failure decision: %+v", decision)
	}
	if decision.FallbackReason != ReviewFallbackReasonNoSelectedCandidate {
		t.Fatalf("expected no_selected_candidate fallback reason, got %+v", decision)
	}
	if !decision.ReviewFlowBuildPresent || decision.ReviewFlowPresent || decision.SequenceModelPresent || decision.MermaidPresent {
		t.Fatalf("unexpected artifact booleans for strict failure: %+v", decision)
	}
}

func TestRenderRootStrictReviewFailsOnVisibleArtifactLeak(t *testing.T) {
	w := testWorkflowWithReviewBuilder(stubReviewFlowBuilder{
		result: invalidSelectedBuildResult(),
	})

	rendered, decision, err := w.renderRoot(
		Request{RenderMode: RenderModeReview, ReviewStrict: true},
		graph.GraphSnapshot{},
		testHTTPRoot(),
		testReducedChain(),
		flow_stitch.SemanticAuditRoot{},
		sequence_model_build.Options{Title: "Review"},
	)
	if err == nil {
		t.Fatal("expected strict review mode to fail on invalid selected output")
	}
	if !strings.Contains(err.Error(), string(ReviewFallbackReasonReviewValidationFailed)) {
		t.Fatalf("expected review_validation_failed error, got %v", err)
	}
	if rendered.reviewFlow == nil || rendered.reviewFlowBuild == nil {
		t.Fatalf("expected strict failure to preserve review artifacts for debug output, got %+v", rendered)
	}
	if !decision.ReviewAttempted || !decision.ReviewSelected || decision.FallbackUsed {
		t.Fatalf("unexpected strict validation decision: %+v", decision)
	}
	if decision.FallbackReason != ReviewFallbackReasonReviewValidationFailed {
		t.Fatalf("expected review_validation_failed fallback reason, got %+v", decision)
	}
}

func TestRenderRootNonStrictReviewFallsBackToReduced(t *testing.T) {
	w := testWorkflowWithReviewBuilder(stubReviewFlowBuilder{
		result: invalidSelectedBuildResult(),
	})

	rendered, decision, err := w.renderRoot(
		Request{RenderMode: RenderModeReview},
		graph.GraphSnapshot{},
		testHTTPRoot(),
		testReducedChain(),
		flow_stitch.SemanticAuditRoot{},
		sequence_model_build.Options{Title: "Review"},
	)
	if err != nil {
		t.Fatalf("expected non-strict review mode to fall back, got %v", err)
	}
	if rendered.reviewFlow != nil || rendered.reviewFlowBuild != nil {
		t.Fatalf("expected fallback render to omit review artifacts, got %+v", rendered)
	}
	if decision.UsedRenderer != UsedRendererReducedChain || !decision.FallbackUsed {
		t.Fatalf("expected reduced fallback decision, got %+v", decision)
	}
	if decision.FallbackReason != ReviewFallbackReasonReviewValidationFailed {
		t.Fatalf("expected review_validation_failed fallback reason, got %+v", decision)
	}
	if !decision.SequenceModelPresent || !decision.MermaidPresent {
		t.Fatalf("expected reduced fallback artifacts, got %+v", decision)
	}
}

func TestDebugBundleWritesRenderDecisionArtifacts(t *testing.T) {
	w := testWorkflowWithReviewBuilder(stubReviewFlowBuilder{
		result: invalidSelectedBuildResult(),
	})

	rendered, decision, err := w.renderRoot(
		Request{RenderMode: RenderModeReview, ReviewStrict: true},
		graph.GraphSnapshot{},
		testHTTPRoot(),
		testReducedChain(),
		flow_stitch.SemanticAuditRoot{},
		sequence_model_build.Options{Title: "Review"},
	)
	if err == nil {
		t.Fatal("expected strict review mode to fail")
	}

	t.Run("single-root", func(t *testing.T) {
		dir := t.TempDir()
		debug := debugBundle{
			dir:                 dir,
			rootRenderDecisions: []RootRenderDecision{decision},
			renderDecision:      &decision,
			reviewFlow:          rendered.reviewFlow,
			reviewFlowBuild:     rendered.reviewFlowBuild,
			sequenceModel:       debugSequenceDiagram(rendered.diagram),
			mermaidCode:         rendered.mermaidCode,
		}
		if err := debug.write(); err != nil {
			t.Fatalf("write debug bundle: %v", err)
		}
		assertRenderDecisionFile(t, filepath.Join(dir, "root_render_decisions.json"), true, decision)
		assertRenderDecisionFile(t, filepath.Join(dir, "render_decision.json"), false, decision)
	})

	t.Run("per-root", func(t *testing.T) {
		dir := t.TempDir()
		debug := debugBundle{
			dir:                 dir,
			rootRenderDecisions: []RootRenderDecision{decision},
			rootRenderOutputs: []rootRenderDebug{
				{
					Slug:           decision.Slug,
					RenderDecision: &decision,
				},
			},
		}
		if err := debug.write(); err != nil {
			t.Fatalf("write debug bundle: %v", err)
		}
		assertRenderDecisionFile(t, filepath.Join(dir, "root_render_decisions.json"), true, decision)
		assertRenderDecisionFile(t, filepath.Join(dir, "roots", decision.Slug, "render_decision.json"), false, decision)
	})
}

type stubReviewFlowBuilder struct {
	result reviewflow_build.BuildResult
	err    error
}

func (s stubReviewFlowBuilder) Build(graph.GraphSnapshot, entrypoint.Root, reduced.Chain, flow_stitch.SemanticAuditRoot) (reviewflow_build.BuildResult, error) {
	return s.result, s.err
}

func testWorkflowWithReviewBuilder(builder reviewFlowBuilder) Workflow {
	return Workflow{
		reviewFlow:    builder,
		sequenceModel: sequence_model_build.New(),
		mermaidEmit:   mermaid_emit.New(),
	}
}

func testHTTPRoot() entrypoint.Root {
	return entrypoint.Root{
		NodeID:        "boundary_process",
		CanonicalName: "POST /process",
		RootType:      entrypoint.RootHTTP,
		Framework:     "gin",
		Method:        "POST",
		Path:          "/process",
	}
}

func testReducedChain() reduced.Chain {
	return reduced.Chain{
		RootNodeID: "boundary_process",
		Nodes: []reduced.Node{
			{ID: "boundary_process", ShortName: "POST /process", CanonicalName: "POST /process", Role: reduced.RoleRoot},
			{ID: "handler_process", ShortName: "Handler", CanonicalName: "main.Handler", Role: reduced.RoleBoundary},
		},
		Edges: []reduced.Edge{
			{FromID: "boundary_process", ToID: "handler_process", Label: "dispatch request", OrderIndex: 0},
		},
	}
}

func validSelectedReviewFlow() reviewflow.Flow {
	return reviewflow.Flow{
		ID:            "reviewflow_selected",
		RootNodeID:    "boundary_process",
		CanonicalName: "POST /process",
		Participants: []reviewflow.Participant{
			{ID: "review_boundary", Kind: "boundary", Label: "POST /process"},
			{ID: "review_handler", Kind: "handler", Label: "Handler"},
		},
		Stages: []reviewflow.Stage{
			{
				ID:             "stage_boundary_entry",
				Kind:           "boundary_entry",
				Label:          "Boundary Entry",
				ParticipantIDs: []string{"review_boundary", "review_handler"},
				Messages: []reviewflow.Message{
					{ID: "msg_dispatch", FromParticipantID: "review_boundary", ToParticipantID: "review_handler", Label: "dispatch request", Kind: reviewflow.MessageSync},
				},
			},
			{
				ID:             "stage_response",
				Kind:           "response",
				Label:          "Response",
				ParticipantIDs: []string{"review_handler", "review_boundary"},
				Messages: []reviewflow.Message{
					{ID: "msg_return", FromParticipantID: "review_handler", ToParticipantID: "review_boundary", Label: "return response", Kind: reviewflow.MessageReturn},
				},
			},
		},
		Metadata: reviewflow.Metadata{
			CandidateKind: string(reviewflow_build.CandidateCompactReview),
			Signature:     "compact_review_signature",
			RootFramework: "gin",
		},
	}
}

func invalidSelectedBuildResult() reviewflow_build.BuildResult {
	selected := validSelectedReviewFlow()
	selected.Participants[1].Label = "$inline_handler_0"
	return reviewflow_build.BuildResult{
		Selected:   selected,
		Candidates: []reviewflow.Flow{selected},
		SelectedID: selected.ID,
		Signature:  selected.Metadata.Signature,
	}
}

func assertRenderDecisionFile(t *testing.T, path string, isArray bool, want RootRenderDecision) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if isArray {
		var decisions []RootRenderDecision
		if err := json.Unmarshal(data, &decisions); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if len(decisions) != 1 {
			t.Fatalf("expected one render decision in %s, got %d", path, len(decisions))
		}
		if decisions[0].FallbackReason != want.FallbackReason || decisions[0].Slug != want.Slug {
			t.Fatalf("unexpected render decision in %s: %+v", path, decisions[0])
		}
		return
	}

	var decision RootRenderDecision
	if err := json.Unmarshal(data, &decision); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if decision.FallbackReason != want.FallbackReason || decision.Slug != want.Slug {
		t.Fatalf("unexpected render decision in %s: %+v", path, decision)
	}
}
