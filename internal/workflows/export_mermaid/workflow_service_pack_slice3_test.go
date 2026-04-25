package export_mermaid

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/reviewpack"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/reviewflow_build"
	"analysis-module/internal/services/reviewflow_policy"
)

func TestBuildReviewFlowWithPolicy_PassesServicePackEntryMode(t *testing.T) {
	builder := &stubReviewFlowOptionsBuilder{}
	w := Workflow{
		reviewFlow: builder,
	}

	_, err := w.buildReviewFlowWithPolicy(
		graph.GraphSnapshot{},
		entrypoint.Root{
			NodeID:        "boundary_config",
			CanonicalName: "GET /config",
			RootType:      entrypoint.RootHTTP,
			Framework:     "gin",
			Method:        "GET",
			Path:          "/config",
		},
		reduced.Chain{RootNodeID: "boundary_config"},
		flow_stitch.SemanticAuditRoot{},
		reviewflow_policy.Policy{
			Family:                   reviewflow_policy.FamilyConfigLookup,
			AddHTTPEntryParticipants: true,
		},
	)
	if err != nil {
		t.Fatalf("buildReviewFlowWithPolicy: %v", err)
	}
	if builder.lastOptions == nil {
		t.Fatalf("expected BuildWithOptions path to be used")
	}
	if builder.lastOptions.EntryMode != reviewflow_build.EntryModeServicePackHTTP {
		t.Fatalf("expected service-pack HTTP entry mode, got %+v", builder.lastOptions)
	}
	if !builder.lastOptions.ExpansionEnabled {
		t.Fatalf("expected expansion to be enabled in service-pack policy mode, got %+v", builder.lastOptions)
	}
	if builder.lastOptions.Policy == nil || builder.lastOptions.Policy.Family != reviewflow_policy.FamilyConfigLookup {
		t.Fatalf("expected policy to be forwarded through options, got %+v", builder.lastOptions)
	}
}

func TestQualityFlagsForRenderedFlow_HTTPPolicySignals(t *testing.T) {
	flow := reviewflow.Flow{
		RootNodeID:     "boundary_predict",
		CanonicalName:  "POST /predict",
		SourceRootType: string(entrypoint.RootHTTP),
		Metadata: reviewflow.Metadata{
			RootFramework: "gin",
		},
		Participants: []reviewflow.Participant{
			{ID: "client", Kind: "client", Label: "Client"},
			{ID: "framework", Kind: "boundary", Label: "Gin"},
			{ID: "handler", Kind: "handler", Label: "Handler"},
			{ID: "worker", Kind: "async_sink", Label: "Async Worker"},
		},
		Stages: []reviewflow.Stage{
			{
				Kind: "boundary_entry",
				Messages: []reviewflow.Message{
					{FromParticipantID: "client", ToParticipantID: "framework", Label: "POST /predict", Kind: reviewflow.MessageSync},
					{FromParticipantID: "framework", ToParticipantID: "handler", Label: "invoke handler", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: "post_processing",
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "handler", Label: "normalize output", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: "deferred_async",
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "worker", Label: "spawn worker", Kind: reviewflow.MessageAsync},
				},
			},
		},
		Blocks: []reviewflow.Block{
			{
				Kind: reviewflow.BlockAlt,
				Sections: []reviewflow.BlockSection{
					{
						Label: "fallback branch",
					},
				},
			},
		},
	}

	flags := qualityFlagsForRenderedFlow(reviewflow_policy.Policy{
		Family:                   reviewflow_policy.FamilyDetectorPipeline,
		AddHTTPEntryParticipants: true,
		PreserveBranchBlocks:     true,
		PreserveAsyncBlocks:      true,
		PreservePostProcessing:   true,
	}, &flow, "", reviewpack.RenderSourceReviewFlow, qualityEvidenceContext{})

	for _, expected := range []string{
		"entry_abstraction_present",
		"branch_expected_present",
		"async_expected_present",
		"post_processing_expected_present",
		"no_visible_artifact_leak",
	} {
		if !containsString(flags, expected) {
			t.Fatalf("expected quality flag %q in %+v", expected, flags)
		}
	}
}

type stubReviewFlowOptionsBuilder struct {
	lastOptions *reviewflow_build.BuildOptions
}

func (s *stubReviewFlowOptionsBuilder) Build(graph.GraphSnapshot, entrypoint.Root, reduced.Chain, flow_stitch.SemanticAuditRoot) (reviewflow_build.BuildResult, error) {
	return stubBuildResult(), nil
}

func (s *stubReviewFlowOptionsBuilder) BuildWithOptions(_ graph.GraphSnapshot, _ entrypoint.Root, _ reduced.Chain, _ flow_stitch.SemanticAuditRoot, options reviewflow_build.BuildOptions) (reviewflow_build.BuildResult, error) {
	cloned := options
	s.lastOptions = &cloned
	return stubBuildResult(), nil
}

func stubBuildResult() reviewflow_build.BuildResult {
	selected := reviewflow.Flow{
		ID:            "selected",
		RootNodeID:    "boundary_config",
		CanonicalName: "GET /config",
		Participants: []reviewflow.Participant{
			{ID: "client", Label: "Client"},
			{ID: "framework", Label: "Gin"},
			{ID: "handler", Label: "Handler"},
		},
		Stages: []reviewflow.Stage{
			{
				Kind: "boundary_entry",
				Messages: []reviewflow.Message{
					{FromParticipantID: "client", ToParticipantID: "framework", Label: "GET /config"},
					{FromParticipantID: "framework", ToParticipantID: "handler", Label: "invoke handler"},
				},
			},
		},
	}
	return reviewflow_build.BuildResult{
		Selected:   selected,
		Candidates: []reviewflow.Flow{selected},
		SelectedID: selected.ID,
		Signature:  "selected_signature",
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
