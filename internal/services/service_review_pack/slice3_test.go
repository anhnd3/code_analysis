package service_review_pack

import (
	"strings"
	"testing"

	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/reviewpack"
)

func TestBuild_SelectedFlowCarriesSlice3Metadata(t *testing.T) {
	svc := New()
	pack, err := svc.Build(BuildInput{
		ServiceName: "svc",
		ExpectedRoots: []reviewpack.ExpectedRoot{
			{
				ID:       "get_config",
				RootType: "http",
				Method:   "GET",
				Path:     "/config",
				Family:   "config_lookup",
				Required: true,
			},
		},
		ResolvedRoots: []entrypoint.Root{
			{NodeID: "root_cfg", RootType: entrypoint.RootHTTP, Method: "GET", Path: "/config", CanonicalName: "GET /config"},
		},
		Outcomes: map[string]RenderOutcome{
			"get_config": {
				ExpectedRootID:    "get_config",
				RootNodeID:        "root_cfg",
				CanonicalName:     "GET /config",
				Status:            reviewpack.CoverageRendered,
				RenderSource:      reviewpack.RenderSourceReviewFlow,
				PolicySource:      reviewpack.PolicySourceManifest,
				Family:            "config_lookup",
				PolicyFamily:      "config_lookup",
				CandidateKind:     "compact_review",
				Signature:         "cfg_signature",
				ParticipantCount:  3,
				StageCount:        2,
				MessageCount:      3,
				QualityFlags:      []string{"entry_abstraction_present", "no_visible_artifact_leak"},
				ArtifactSlug:      "get_config",
				MermaidPath:       "flows/get_config.mmd",
				ReviewFlowPath:    "flows/get_config__review_flow.json",
				SequenceModelPath: "flows/get_config__sequence_model.json",
			},
		},
	})
	if err != nil {
		t.Fatalf("build pack: %v", err)
	}
	if len(pack.SelectedFlows) != 1 {
		t.Fatalf("expected one selected flow, got %d", len(pack.SelectedFlows))
	}
	selected := pack.SelectedFlows[0]
	if selected.PolicyFamily != "config_lookup" {
		t.Fatalf("expected policy family to be populated, got %+v", selected)
	}
	if selected.CandidateKind != "compact_review" || selected.Signature != "cfg_signature" {
		t.Fatalf("expected candidate metadata to be populated, got %+v", selected)
	}
	if selected.ParticipantCount != 3 || selected.StageCount != 2 || selected.MessageCount != 3 {
		t.Fatalf("expected count metadata to be populated, got %+v", selected)
	}
	if len(selected.QualityFlags) != 2 {
		t.Fatalf("expected quality flags to be populated, got %+v", selected)
	}
}

func TestMarkdown_IncludesSlice3QualityChecklist(t *testing.T) {
	pack := reviewpack.ServiceReviewPack{
		ServiceName: "svc",
		Coverage: []reviewpack.CoverageItem{
			{
				ExpectedRootID:   "predict",
				RootType:         "http",
				Status:           reviewpack.CoverageRendered,
				Required:         true,
				RequiredBlocking: true,
				RenderSource:     reviewpack.RenderSourceReviewFlow,
			},
		},
		SelectedFlows: []reviewpack.SelectedFlow{
			{
				ExpectedRootID:   "predict",
				CanonicalName:    "POST /predict",
				PolicyFamily:     "detector_pipeline",
				PolicySource:     reviewpack.PolicySourceManifest,
				RenderSource:     reviewpack.RenderSourceReviewFlow,
				CandidateKind:    "faithful",
				Signature:        "predict_signature",
				ParticipantCount: 7,
				StageCount:       4,
				MessageCount:     10,
				QualityFlags: []string{
					"entry_abstraction_present",
					"branch_expected_missing",
					"async_expected_missing",
					"post_processing_expected_missing",
					"no_visible_artifact_leak",
				},
				MermaidPath: "flows/predict.mmd",
			},
		},
	}

	md := Markdown(pack)
	if !strings.Contains(md, "| expected_root_id | policy_family | candidate_kind | entry_ok | branch_ok | async_ok | post_processing_ok | no_leak | verdict |") {
		t.Fatalf("expected quality checklist table header, got:\n%s", md)
	}
	if !strings.Contains(md, "| predict | detector_pipeline | faithful | yes | no | no | no | yes | needs_slice4 |") {
		t.Fatalf("expected needs_slice4 checklist row, got:\n%s", md)
	}
}
