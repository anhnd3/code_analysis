package export_mermaid

import (
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/reviewpack"
	"analysis-module/internal/services/reviewflow_expand"
	"analysis-module/internal/services/reviewflow_policy"
)

func TestQualityFlagsForRenderedFlow_ClassBasedEvidenceTurnsExpectedFlagsPresent(t *testing.T) {
	flow := reviewflow.Flow{
		RootNodeID:     "boundary_predict",
		CanonicalName:  "POST /scan360/v1/predict",
		SourceRootType: string(entrypoint.RootHTTP),
		Metadata:       reviewflow.Metadata{RootFramework: "gin"},
		Participants: []reviewflow.Participant{
			{ID: "client", Kind: "client", Label: "Client"},
			{ID: "framework", Kind: "boundary", Label: "Gin"},
			{ID: "handler", Kind: "handler", Label: "Handler"},
			{ID: "processor", Kind: "processor", Label: "Processor"},
			{ID: "worker", Kind: "async_sink", Label: "Async Worker"},
		},
		Stages: []reviewflow.Stage{
			{
				Kind: "boundary_entry",
				Messages: []reviewflow.Message{
					{FromParticipantID: "client", ToParticipantID: "framework", Label: "POST /scan360/v1/predict", Kind: reviewflow.MessageSync},
					{FromParticipantID: "framework", ToParticipantID: "handler", Label: "invoke handler", Kind: reviewflow.MessageSync},
				},
			},
			{
				Kind: "business_core",
				Messages: []reviewflow.Message{
					{FromParticipantID: "handler", ToParticipantID: "processor", Label: "processor selection", Kind: reviewflow.MessageSync, Class: reviewflow_expand.ClassBranchBusiness},
					{FromParticipantID: "handler", ToParticipantID: "worker", Label: "spawn worker", Kind: reviewflow.MessageAsync, Class: reviewflow_expand.ClassAsyncWorker},
					{FromParticipantID: "processor", ToParticipantID: "handler", Label: "post process results", Kind: reviewflow.MessageSync, Class: reviewflow_expand.ClassPostProcessing},
				},
			},
		},
	}

	flags := qualityFlagsForRenderedFlow(reviewflow_policy.Policy{
		Family:                   reviewflow_policy.FamilyScanPipeline,
		AddHTTPEntryParticipants: true,
		PreserveBranchBlocks:     true,
		PreserveAsyncBlocks:      true,
		PreservePostProcessing:   true,
	}, &flow, "", reviewpack.RenderSourceReviewFlow)

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

	for _, missing := range []string{
		"branch_expected_missing",
		"async_expected_missing",
		"post_processing_expected_missing",
	} {
		if containsString(flags, missing) {
			t.Fatalf("did not expect %q when class-based evidence is present, got %+v", missing, flags)
		}
	}
}
