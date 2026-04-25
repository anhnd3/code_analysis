package service_review_pack

import (
	"testing"

	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/reviewpack"
)

func TestBuild_UnsupportedRootTypeIsSkippedSelection(t *testing.T) {
	svc := New()
	pack, err := svc.Build(BuildInput{
		ServiceName: "svc",
		ExpectedRoots: []reviewpack.ExpectedRoot{
			{
				ID:       "start_service",
				RootType: "bootstrap",
				Required: true,
				Family:   "bootstrap_startup",
			},
		},
		ResolvedRoots: []entrypoint.Root{
			{NodeID: "root_bootstrap", RootType: entrypoint.RootBootstrap, CanonicalName: "main.main"},
		},
		Outcomes: map[string]RenderOutcome{
			"start_service": {
				ExpectedRootID: "start_service",
				RootNodeID:     "root_bootstrap",
				Status:         reviewpack.CoverageSkipped,
				Reason:         reviewpack.ReasonUnsupportedRootType,
				FailureStage:   reviewpack.FailureStageSelection,
			},
		},
	})
	if err != nil {
		t.Fatalf("build pack: %v", err)
	}
	if len(pack.Coverage) != 1 {
		t.Fatalf("expected one coverage item, got %d", len(pack.Coverage))
	}
	item := pack.Coverage[0]
	if item.Status != reviewpack.CoverageSkipped {
		t.Fatalf("expected skipped, got %s", item.Status)
	}
	if item.FailureStage != reviewpack.FailureStageSelection {
		t.Fatalf("expected selection failure stage, got %s", item.FailureStage)
	}
	if item.Reason != reviewpack.ReasonUnsupportedRootType {
		t.Fatalf("expected unsupported_root_type, got %s", item.Reason)
	}
	if !item.RequiredBlocking {
		t.Fatalf("expected required root to be blocking")
	}
}

func TestBuild_OptionalUnresolvedIsNonBlocking(t *testing.T) {
	svc := New()
	pack, err := svc.Build(BuildInput{
		ServiceName: "svc",
		ExpectedRoots: []reviewpack.ExpectedRoot{
			{
				ID:       "optional_cfg",
				RootType: "http",
				Method:   "GET",
				Path:     "/cfg",
				Required: false,
				Family:   "config_lookup",
			},
		},
		DetectedBoundaries: []boundaryroot.Root{
			{
				Kind:   boundaryroot.KindHTTP,
				Method: "GET",
				Path:   "/cfg",
			},
		},
	})
	if err != nil {
		t.Fatalf("build pack: %v", err)
	}
	item := pack.Coverage[0]
	if item.RequiredBlocking {
		t.Fatalf("optional root should not be blocking")
	}
	if item.Status != reviewpack.CoverageMissing {
		t.Fatalf("expected missing coverage, got %s", item.Status)
	}
	if item.Reason != reviewpack.ReasonRootNotResolved {
		t.Fatalf("expected root_not_resolved, got %s", item.Reason)
	}
	if item.FailureStage != reviewpack.FailureStageResolution {
		t.Fatalf("expected resolution stage, got %s", item.FailureStage)
	}
}

func TestBuild_RenderedClearsFailureMetadata(t *testing.T) {
	svc := New()
	pack, err := svc.Build(BuildInput{
		ServiceName: "svc",
		ExpectedRoots: []reviewpack.ExpectedRoot{
			{
				ID:       "predict",
				RootType: "http",
				Method:   "POST",
				Path:     "/predict",
				Required: true,
				Family:   "scan_pipeline",
			},
		},
		ResolvedRoots: []entrypoint.Root{
			{NodeID: "root_predict", RootType: entrypoint.RootHTTP, Method: "POST", Path: "/predict", CanonicalName: "POST /predict"},
		},
		Outcomes: map[string]RenderOutcome{
			"predict": {
				ExpectedRootID: "predict",
				RootNodeID:     "root_predict",
				CanonicalName:  "POST /predict",
				Status:         reviewpack.CoverageRendered,
				Reason:         reviewpack.ReasonReviewRenderFailed,
				FailureStage:   reviewpack.FailureStageRendering,
			},
		},
	})
	if err != nil {
		t.Fatalf("build pack: %v", err)
	}
	item := pack.Coverage[0]
	if item.Status != reviewpack.CoverageRendered {
		t.Fatalf("expected rendered, got %s", item.Status)
	}
	if item.Reason != "" {
		t.Fatalf("rendered item should not keep reason, got %s", item.Reason)
	}
	if item.FailureStage != "" {
		t.Fatalf("rendered item should not keep failure stage, got %s", item.FailureStage)
	}
}

func TestBuild_BootstrapPrefersHighConfidenceMainRoot(t *testing.T) {
	svc := New()
	pack, err := svc.Build(BuildInput{
		ServiceName: "svc",
		ExpectedRoots: []reviewpack.ExpectedRoot{
			{
				ID:       "start_service",
				RootType: "bootstrap",
				Required: true,
				Family:   "bootstrap_startup",
			},
		},
		ResolvedRoots: []entrypoint.Root{
			{
				NodeID:        "root_execute",
				CanonicalName: "cmd.Execute",
				RootType:      entrypoint.RootBootstrap,
				Confidence:    entrypoint.ConfidenceMedium,
			},
			{
				NodeID:        "root_main",
				CanonicalName: "main.main",
				RootType:      entrypoint.RootBootstrap,
				Confidence:    entrypoint.ConfidenceHigh,
			},
		},
	})
	if err != nil {
		t.Fatalf("build pack: %v", err)
	}
	if len(pack.Coverage) != 1 {
		t.Fatalf("expected one coverage item, got %d", len(pack.Coverage))
	}
	if pack.Coverage[0].RootNodeID != "root_main" {
		t.Fatalf("expected high-confidence main root, got %s", pack.Coverage[0].RootNodeID)
	}
}

func TestBuild_BootstrapRenderedFlowCarriesLifecycleMetadata(t *testing.T) {
	svc := New()
	pack, err := svc.Build(BuildInput{
		ServiceName: "svc",
		ExpectedRoots: []reviewpack.ExpectedRoot{
			{
				ID:       "start_service",
				RootType: "bootstrap",
				Required: true,
				Family:   "bootstrap_startup",
			},
		},
		ResolvedRoots: []entrypoint.Root{
			{NodeID: "root_main", RootType: entrypoint.RootBootstrap, CanonicalName: "main.main"},
		},
		Outcomes: map[string]RenderOutcome{
			"start_service": {
				ExpectedRootID:    "start_service",
				RootNodeID:        "root_main",
				CanonicalName:     "main.main",
				Status:            reviewpack.CoverageRendered,
				RenderSource:      reviewpack.RenderSourceBootstrapLifecycle,
				PolicySource:      reviewpack.PolicySourceManifest,
				Family:            "bootstrap_startup",
				PolicyFamily:      "bootstrap_startup",
				CandidateKind:     "bootstrap_lifecycle",
				Signature:         "bootstrap_signature",
				ParticipantCount:  2,
				StageCount:        4,
				MessageCount:      4,
				QualityFlags:      []string{"no_visible_artifact_leak"},
				ArtifactSlug:      "start_service",
				MermaidPath:       "flows/start_service.mmd",
				ReviewFlowPath:    "flows/start_service__review_flow.json",
				SequenceModelPath: "flows/start_service__sequence_model.json",
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
	if selected.RenderSource != reviewpack.RenderSourceBootstrapLifecycle {
		t.Fatalf("expected bootstrap lifecycle render source, got %+v", selected)
	}
	if selected.PolicySource != reviewpack.PolicySourceManifest {
		t.Fatalf("expected manifest policy source, got %+v", selected)
	}
	if selected.CandidateKind != "bootstrap_lifecycle" {
		t.Fatalf("expected bootstrap lifecycle candidate kind, got %+v", selected)
	}
}
