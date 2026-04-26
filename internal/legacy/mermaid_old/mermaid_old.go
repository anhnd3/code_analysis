package mermaid_old

import (
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	"analysis-module/internal/services/boundary_detect"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/cross_boundary_link"
	"analysis-module/internal/services/entrypoint_resolve"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/mermaid_emit"
	"analysis-module/internal/services/sequence_model_build"
	oldworkflow "analysis-module/internal/workflows/export_mermaid"
)

// Package mermaid_old is the compatibility boundary for the legacy deterministic Mermaid exporter.

type RootTypeFilter = oldworkflow.RootTypeFilter
type RootExportStatus = oldworkflow.RootExportStatus
type RenderMode = oldworkflow.RenderMode
type ReviewScope = oldworkflow.ReviewScope
type UsedRenderer = oldworkflow.UsedRenderer
type ReviewFallbackReason = oldworkflow.ReviewFallbackReason

type RootRenderDecision = oldworkflow.RootRenderDecision
type RootExport = oldworkflow.RootExport
type Request = oldworkflow.Request
type Result = oldworkflow.Result
type Workflow = oldworkflow.Workflow

const (
	RootFilterBootstrap = oldworkflow.RootFilterBootstrap
	RootFilterHTTP      = oldworkflow.RootFilterHTTP
	RootFilterWorker    = oldworkflow.RootFilterWorker
	RootFilterSymbol    = oldworkflow.RootFilterSymbol
	RootFilterMaster    = oldworkflow.RootFilterMaster

	RootExportRendered = oldworkflow.RootExportRendered
	RootExportSkipped  = oldworkflow.RootExportSkipped

	RenderModeAuto         = oldworkflow.RenderModeAuto
	RenderModeReview       = oldworkflow.RenderModeReview
	RenderModeReducedDebug = oldworkflow.RenderModeReducedDebug

	ReviewScopeRoot        = oldworkflow.ReviewScopeRoot
	ReviewScopeServicePack = oldworkflow.ReviewScopeServicePack

	UsedRendererReviewFlow   = oldworkflow.UsedRendererReviewFlow
	UsedRendererReducedChain = oldworkflow.UsedRendererReducedChain

	ReviewFallbackReasonNone                      = oldworkflow.ReviewFallbackReasonNone
	ReviewFallbackReasonNoSelectedCandidate       = oldworkflow.ReviewFallbackReasonNoSelectedCandidate
	ReviewFallbackReasonIncompleteReviewArtifacts = oldworkflow.ReviewFallbackReasonIncompleteReviewArtifacts
	ReviewFallbackReasonReviewValidationFailed    = oldworkflow.ReviewFallbackReasonReviewValidationFailed
	ReviewFallbackReasonReviewBuildEmpty          = oldworkflow.ReviewFallbackReasonReviewBuildEmpty
	ReviewFallbackReasonReviewRenderError         = oldworkflow.ReviewFallbackReasonReviewRenderError
)

func New(
	artifactStore artifactstoreport.Store,
	entrypointResolveSvc entrypoint_resolve.Service,
	flowStitchSvc flow_stitch.Service,
	crossBoundaryLinkSvc cross_boundary_link.Service,
	chainReduceSvc chain_reduce.Service,
	sequenceModelSvc sequence_model_build.Service,
	mermaidEmitSvc mermaid_emit.Service,
	boundaryDetectSvc boundary_detect.Service,
) Workflow {
	return oldworkflow.New(
		artifactStore,
		entrypointResolveSvc,
		flowStitchSvc,
		crossBoundaryLinkSvc,
		chainReduceSvc,
		sequenceModelSvc,
		mermaidEmitSvc,
		boundaryDetectSvc,
	)
}
