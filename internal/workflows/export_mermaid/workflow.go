package export_mermaid

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/sequence"
	"analysis-module/internal/domain/symbol"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	"analysis-module/internal/services/boundary_detect"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/cross_boundary_link"
	"analysis-module/internal/services/entrypoint_resolve"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/mermaid_emit"
	"analysis-module/internal/services/reviewflow_build"
	"analysis-module/internal/services/sequence_model_build"
)

// RootTypeFilter selects which entrypoint types to include.
type RootTypeFilter string

const (
	RootFilterBootstrap RootTypeFilter = "bootstrap"
	RootFilterHTTP      RootTypeFilter = "http"
	RootFilterWorker    RootTypeFilter = "worker"
	RootFilterSymbol    RootTypeFilter = "symbol"
	RootFilterMaster    RootTypeFilter = "master"
)

type RootExportStatus string

const (
	RootExportRendered RootExportStatus = "rendered"
	RootExportSkipped  RootExportStatus = "skipped"
)

type RenderMode string

const (
	RenderModeAuto         RenderMode = "auto"
	RenderModeReview       RenderMode = "review"
	RenderModeReducedDebug RenderMode = "reduced_debug"
)

type ReviewScope string

const (
	ReviewScopeRoot        ReviewScope = "root"
	ReviewScopeServicePack ReviewScope = "service_pack"
)

type UsedRenderer string

const (
	UsedRendererReviewFlow   UsedRenderer = "reviewflow"
	UsedRendererReducedChain UsedRenderer = "reduced_chain"
)

type ReviewFallbackReason string

const (
	ReviewFallbackReasonNone                      ReviewFallbackReason = ""
	ReviewFallbackReasonNoSelectedCandidate       ReviewFallbackReason = "no_selected_candidate"
	ReviewFallbackReasonIncompleteReviewArtifacts ReviewFallbackReason = "incomplete_review_artifacts"
	ReviewFallbackReasonReviewValidationFailed    ReviewFallbackReason = "review_validation_failed"
	ReviewFallbackReasonReviewBuildEmpty          ReviewFallbackReason = "review_build_empty"
	ReviewFallbackReasonReviewRenderError         ReviewFallbackReason = "review_render_error"
)

// RootRenderDecision records which renderer path was attempted and which one produced output.
type RootRenderDecision struct {
	RootNodeID             string               `json:"root_node_id"`
	CanonicalName          string               `json:"canonical_name"`
	Slug                   string               `json:"slug"`
	RequestedMode          RenderMode           `json:"requested_mode"`
	EffectiveMode          RenderMode           `json:"effective_mode"`
	UsedRenderer           UsedRenderer         `json:"used_renderer"`
	ReviewAttempted        bool                 `json:"review_attempted"`
	ReviewSelected         bool                 `json:"review_selected"`
	FallbackUsed           bool                 `json:"fallback_used"`
	FallbackReason         ReviewFallbackReason `json:"fallback_reason"`
	SelectedCandidateID    string               `json:"selected_candidate_id"`
	SelectedCandidateKind  string               `json:"selected_candidate_kind"`
	SelectedSignature      string               `json:"selected_signature"`
	CandidateCount         int                  `json:"candidate_count"`
	ReviewFlowPresent      bool                 `json:"review_flow_present"`
	ReviewFlowBuildPresent bool                 `json:"review_flow_build_present"`
	SequenceModelPresent   bool                 `json:"sequence_model_present"`
	MermaidPresent         bool                 `json:"mermaid_present"`
}

// RootExport records the render outcome for a single resolved root.
type RootExport struct {
	RootNodeID    string           `json:"root_node_id"`
	CanonicalName string           `json:"canonical_name"`
	Slug          string           `json:"slug"`
	Status        RootExportStatus `json:"status"`
	Reason        string           `json:"reason,omitempty"`
	ArtifactRefs  []artifact.Ref   `json:"artifact_refs,omitempty"`
}

// Request configures the export_mermaid workflow.
type Request struct {
	WorkspaceID       string         `json:"workspace_id"`
	SnapshotID        string         `json:"snapshot_id"`
	RootType          RootTypeFilter `json:"root_type"`
	RootSelector      string         `json:"root_selector,omitempty"`
	ReviewScope       ReviewScope    `json:"review_scope,omitempty"`
	ExpectedRootsFile string         `json:"expected_roots_file,omitempty"`
	RenderMode        RenderMode     `json:"render_mode,omitempty"`
	ReviewStrict      bool           `json:"review_strict,omitempty"`
	MaxDepth          int            `json:"max_depth,omitempty"`
	MaxBranches       int            `json:"max_branches,omitempty"`
	CollapseMode      string         `json:"collapse_mode,omitempty"`
	ServiceShortName  string         `json:"service_short_name,omitempty"`
	IncludeCandidates bool           `json:"include_candidates,omitempty"`
	DebugBundleDir    string         `json:"debug_bundle_dir,omitempty"`
}

// Result holds the workflow output.
type Result struct {
	WorkspaceID  string                     `json:"workspace_id"`
	SnapshotID   string                     `json:"snapshot_id"`
	ArtifactRefs []artifact.Ref             `json:"artifact_refs"`
	RootExports  []RootExport               `json:"root_exports,omitempty"`
	FlowMetrics  quality.FlowQualityMetrics `json:"flow_metrics"`
	MermaidCode  string                     `json:"mermaid_code,omitempty"`
}

type reviewFlowBuilder interface {
	Build(snapshot graph.GraphSnapshot, root entrypoint.Root, chain reduced.Chain, audit flow_stitch.SemanticAuditRoot) (reviewflow_build.BuildResult, error)
}

// Workflow orchestrates the full Mermaid export pipeline.
type Workflow struct {
	artifactStore     artifactstoreport.Store
	entrypointResolve entrypoint_resolve.Service
	flowStitch        flow_stitch.Service
	crossBoundaryLink cross_boundary_link.Service
	chainReduce       chain_reduce.Service
	sequenceModel     sequence_model_build.Service
	mermaidEmit       mermaid_emit.Service
	boundaryDetect    boundary_detect.Service
	reviewFlow        reviewFlowBuilder
}

// New creates the export_mermaid workflow.
func New(
	artifactStore artifactstoreport.Store,
	entrypointResolve entrypoint_resolve.Service,
	flowStitch flow_stitch.Service,
	crossBoundaryLink cross_boundary_link.Service,
	chainReduce chain_reduce.Service,
	sequenceModel sequence_model_build.Service,
	mermaidEmit mermaid_emit.Service,
	boundaryDetect boundary_detect.Service,
) Workflow {
	return Workflow{
		artifactStore:     artifactStore,
		entrypointResolve: entrypointResolve,
		flowStitch:        flowStitch,
		crossBoundaryLink: crossBoundaryLink,
		chainReduce:       chainReduce,
		sequenceModel:     sequenceModel,
		mermaidEmit:       mermaidEmit,
		boundaryDetect:    boundaryDetect,
		reviewFlow:        reviewflow_build.New(),
	}
}

// Run executes the complete Mermaid export pipeline.
func (w Workflow) Run(req Request, inventory repository.Inventory, snapshot graph.GraphSnapshot) (Result, error) {
	debug := debugBundle{dir: req.DebugBundleDir}

	var symbols []symbol.Symbol
	for _, node := range snapshot.Nodes {
		if node.Kind == graph.NodeSymbol && node.Location != nil {
			symbols = append(symbols, symbol.Symbol{
				ID:           symbol.ID(node.ID),
				RepositoryID: node.RepositoryID,
				FilePath:     node.FilePath,
				Location:     *node.Location,
			})
		}
	}

	detected, err := w.boundaryDetect.DetectAllDetailed(inventory, symbols)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: detect boundaries: %w", err)
	}
	debug.boundaryRoots = detected.Roots
	debug.boundaryDiagnostics = detected.Diagnostics
	if err := debug.write(); err != nil {
		return Result{}, fmt.Errorf("export_mermaid: write debug bundle: %w", err)
	}

	resolved, err := w.entrypointResolve.Resolve(snapshot, inventory, detected.Roots)
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: resolve entrypoints: %w", err)
	}
	filtered := filterRoots(resolved, req)
	debug.resolvedRoots = filtered
	if err := debug.write(); err != nil {
		return Result{}, fmt.Errorf("export_mermaid: write debug bundle: %w", err)
	}
	if err := ensureNonEmptyRoots(filtered, req.RootType); err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: %w", err)
	}
	if req.ReviewScope == ReviewScopeServicePack {
		return w.runServicePackExport(req, snapshot, inventory, detected.Roots, filtered, debug)
	}

	bundle, err := w.flowStitch.Build(snapshot, filtered, inventory)
	debug.flowBundle = &bundle
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: stitch flows: %w", err)
	}
	if err := ensureNonEmptyChains(bundle); err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: %w", err)
	}
	semanticAudit := w.flowStitch.BuildAudit(snapshot, filtered, bundle)
	debug.semanticAudit = &semanticAudit

	links, err := w.crossBoundaryLink.Build(snapshot, inventory, bundle)
	debug.boundaryBundle = &links
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: link boundaries: %w", err)
	}

	if usesPerRootHTTPExports(req, filtered) {
		return w.runPerRootHTTPExports(req, snapshot, filtered, bundle, links, debug)
	}
	return w.runSingleRootExport(req, snapshot, filtered, bundle, links, debug)
}

func (w Workflow) runSingleRootExport(req Request, snapshot graph.GraphSnapshot, filtered entrypoint.Result, bundle flow.Bundle, links boundary.Bundle, debug debugBundle) (Result, error) {
	reducedChain, err := w.chainReduce.Reduce(snapshot, bundle, links, chain_reduce.Request{
		MaxDepth:     req.MaxDepth,
		MaxBranches:  req.MaxBranches,
		CollapseMode: req.CollapseMode,
	})
	debug.reducedChain = &reducedChain
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: reduce chain: %w", err)
	}
	if err := ensureNonEmptyReducedChain(reducedChain); err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: %w", err)
	}

	root := selectedRoot(filtered, reducedChain.RootNodeID)
	if root.NodeID == "" && len(filtered.Roots) > 0 {
		root = filtered.Roots[0]
	}

	auditRoot, _ := semanticAuditRootByNodeID(debug.semanticAudit, root.NodeID)
	renderedRoot, decision, err := w.renderRoot(req, snapshot, root, reducedChain, auditRoot, sequence_model_build.Options{
		Title:            diagramTitle(req),
		ServiceShortName: req.ServiceShortName,
	})
	debug.renderDecision = &decision
	debug.rootRenderDecisions = []RootRenderDecision{decision}
	debug.sequenceModel = debugSequenceDiagram(renderedRoot.diagram)
	debug.reviewFlow = renderedRoot.reviewFlow
	debug.reviewFlowBuild = renderedRoot.reviewFlowBuild
	debug.mermaidCode = renderedRoot.mermaidCode
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: render root: %w", err)
	}
	diagram := renderedRoot.diagram
	if err := ensureNonEmptySequence(diagram); err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: %w", err)
	}

	mermaidCode := renderedRoot.mermaidCode

	var artifactRefs []artifact.Ref
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "flow_bundle.json", artifact.TypeFlowBundle, bundle); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "boundary_bundle.json", artifact.TypeBoundaryBundle, links); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}

	rootExport := RootExport{
		RootNodeID:    root.NodeID,
		CanonicalName: root.CanonicalName,
		Slug:          rootExportSlug(root),
		Status:        RootExportRendered,
	}
	rootArtifactRefs, err := w.saveSingleRenderArtifacts(req, reducedChain, renderedRoot.reviewFlow, renderedRoot.reviewFlowBuild, diagram, mermaidCode)
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: save render artifacts: %w", err)
	}
	rootExport.ArtifactRefs = manifestArtifactRefs(req.WorkspaceID, rootArtifactRefs)
	artifactRefs = append(artifactRefs, rootArtifactRefs...)
	rootExports := []RootExport{rootExport}

	debug.rootExports = rootExports
	if auditRoot, ok := semanticAuditRootByNodeID(debug.semanticAudit, root.NodeID); ok {
		debug.rootSemanticAudits = append(debug.rootSemanticAudits, rootSemanticAuditDebug{Audit: auditRoot})
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "root_exports.json", artifact.TypeRootExports, rootExports); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if err := debug.write(); err != nil {
		return Result{}, fmt.Errorf("export_mermaid: write debug bundle: %w", err)
	}

	metrics := buildFlowMetrics(filtered, bundle, links, rootExports)
	return Result{
		WorkspaceID:  req.WorkspaceID,
		SnapshotID:   req.SnapshotID,
		ArtifactRefs: artifactRefs,
		RootExports:  rootExports,
		FlowMetrics:  metrics,
		MermaidCode:  mermaidCode,
	}, nil
}

func (w Workflow) runPerRootHTTPExports(req Request, snapshot graph.GraphSnapshot, filtered entrypoint.Result, bundle flow.Bundle, links boundary.Bundle, debug debugBundle) (Result, error) {
	chainByRoot := mapChainsByRoot(bundle.Chains)

	rootExports := make([]RootExport, 0, len(filtered.Roots))
	renderedOutputs := make([]rootRenderOutput, 0, len(filtered.Roots))
	for _, root := range filtered.Roots {
		export := RootExport{
			RootNodeID:    root.NodeID,
			CanonicalName: root.CanonicalName,
			Slug:          rootExportSlug(root),
		}

		chain, ok := chainByRoot[root.NodeID]
		if !ok {
			export.Status = RootExportSkipped
			export.Reason = "no stitched chain for root"
			rootExports = append(rootExports, export)
			continue
		}

		reducedChain, err := w.chainReduce.ReduceChain(snapshot, chain, bundle.BoundaryMarkers, links, chain_reduce.Request{
			MaxDepth:     req.MaxDepth,
			MaxBranches:  req.MaxBranches,
			CollapseMode: req.CollapseMode,
		})
		if err != nil {
			_ = debug.write()
			return Result{}, fmt.Errorf("export_mermaid: reduce chain for %s: %w", root.CanonicalName, err)
		}
		if reducedChain.RootNodeID == "" {
			export.Status = RootExportSkipped
			export.Reason = "reduced chain is empty"
			rootExports = append(rootExports, export)
			continue
		}

		auditRoot, _ := semanticAuditRootByNodeID(debug.semanticAudit, root.NodeID)
		rendered, decision, err := w.renderRoot(req, snapshot, root, reducedChain, auditRoot, sequence_model_build.Options{
			Title:            diagramTitleForRoot(req, root),
			ServiceShortName: req.ServiceShortName,
		})
		debug.rootRenderDecisions = append(debug.rootRenderDecisions, decision)
		if err != nil {
			debug.rootRenderOutputs = append(debug.rootRenderOutputs, rootRenderDebug{
				Slug:            export.Slug,
				ReducedChain:    &reducedChain,
				ReviewFlow:      rendered.reviewFlow,
				ReviewFlowBuild: rendered.reviewFlowBuild,
				Sequence:        debugSequenceDiagram(rendered.diagram),
				MermaidCode:     rendered.mermaidCode,
				RenderDecision:  &decision,
			})
			if auditRoot, ok := semanticAuditRootByNodeID(debug.semanticAudit, root.NodeID); ok {
				debug.rootSemanticAudits = append(debug.rootSemanticAudits, rootSemanticAuditDebug{
					Slug:  export.Slug,
					Audit: auditRoot,
				})
			}
			_ = debug.write()
			return Result{}, fmt.Errorf("export_mermaid: render root for %s: %w", root.CanonicalName, err)
		}
		diagram := rendered.diagram
		if len(diagram.Participants) == 0 && len(diagram.Elements) == 0 {
			export.Status = RootExportSkipped
			export.Reason = "sequence model is empty"
			rootExports = append(rootExports, export)
			continue
		}

		export.Status = RootExportRendered
		rootExports = append(rootExports, export)
		renderedOutputs = append(renderedOutputs, rootRenderOutput{
			exportIndex:     len(rootExports) - 1,
			reducedChain:    reducedChain,
			reviewFlow:      rendered.reviewFlow,
			reviewFlowBuild: rendered.reviewFlowBuild,
			sequence:        diagram,
			mermaidCode:     rendered.mermaidCode,
			renderDecision:  decision,
		})
	}

	debug.rootExports = rootExports
	if renderedRootCount(rootExports) == 0 {
		if err := debug.write(); err != nil {
			return Result{}, fmt.Errorf("export_mermaid: write debug bundle: %w", err)
		}
		return Result{}, fmt.Errorf("export_mermaid: no http roots produced renderable diagrams")
	}

	var artifactRefs []artifact.Ref
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "flow_bundle.json", artifact.TypeFlowBundle, bundle); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "boundary_bundle.json", artifact.TypeBoundaryBundle, links); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}

	for _, output := range renderedOutputs {
		export := &rootExports[output.exportIndex]
		refs, err := w.savePerRootArtifacts(req, export.Slug, output.reducedChain, output.reviewFlow, output.reviewFlowBuild, output.sequence, output.mermaidCode)
		if err != nil {
			_ = debug.write()
			return Result{}, fmt.Errorf("export_mermaid: save per-root artifacts for %s: %w", export.CanonicalName, err)
		}
		export.ArtifactRefs = manifestArtifactRefs(req.WorkspaceID, refs)
		artifactRefs = append(artifactRefs, refs...)
		debug.rootRenderOutputs = append(debug.rootRenderOutputs, rootRenderDebug{
			Slug:            export.Slug,
			ReducedChain:    &output.reducedChain,
			ReviewFlow:      output.reviewFlow,
			ReviewFlowBuild: output.reviewFlowBuild,
			Sequence:        &output.sequence,
			MermaidCode:     output.mermaidCode,
			RenderDecision:  &output.renderDecision,
		})
		if auditRoot, ok := semanticAuditRootByNodeID(debug.semanticAudit, export.RootNodeID); ok {
			debug.rootSemanticAudits = append(debug.rootSemanticAudits, rootSemanticAuditDebug{
				Slug:  export.Slug,
				Audit: auditRoot,
			})
		}
	}

	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "root_exports.json", artifact.TypeRootExports, rootExports); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if err := debug.write(); err != nil {
		return Result{}, fmt.Errorf("export_mermaid: write debug bundle: %w", err)
	}

	metrics := buildFlowMetrics(filtered, bundle, links, rootExports)
	return Result{
		WorkspaceID:  req.WorkspaceID,
		SnapshotID:   req.SnapshotID,
		ArtifactRefs: artifactRefs,
		RootExports:  rootExports,
		FlowMetrics:  metrics,
		MermaidCode:  "",
	}, nil
}

func (w Workflow) saveSingleRenderArtifacts(req Request, reducedChain reduced.Chain, rf *reviewflow.Flow, rfBuild *reviewflow_build.BuildResult, diagram sequence.Diagram, mermaidCode string) ([]artifact.Ref, error) {
	var refs []artifact.Ref
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "reduced_chain.json", artifact.TypeReducedChain, reducedChain); err != nil {
		return nil, err
	} else {
		refs = append(refs, ref)
	}
	if rf != nil {
		if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "review_flow.json", artifact.TypeReviewFlow, rf); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	if rfBuild != nil {
		if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "review_flow_build.json", artifact.TypeReviewFlowBuild, rfBuild); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "sequence_model.json", artifact.TypeSequenceModel, diagram); err != nil {
		return nil, err
	} else {
		refs = append(refs, ref)
	}
	if mermaidCode != "" {
		if ref, err := w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, mermaidFilename(req), artifact.TypeMermaidDiagram, mermaidCode); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	if req.IncludeCandidates && rfBuild != nil {
		candidateRefs, err := w.saveCandidateRenderArtifacts(req, "", *rfBuild, diagram.Title)
		if err != nil {
			return nil, err
		}
		refs = append(refs, candidateRefs...)
	}
	return refs, nil
}

func (w Workflow) savePerRootArtifacts(req Request, slug string, reducedChain reduced.Chain, rf *reviewflow.Flow, rfBuild *reviewflow_build.BuildResult, diagram sequence.Diagram, mermaidCode string) ([]artifact.Ref, error) {
	var refs []artifact.Ref
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, reducedChainFilenameForRoot(slug), artifact.TypeReducedChain, reducedChain); err != nil {
		return nil, err
	} else {
		refs = append(refs, ref)
	}
	if rf != nil {
		if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, reviewFlowFilenameForRoot(slug), artifact.TypeReviewFlow, rf); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	if rfBuild != nil {
		if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, reviewFlowBuildFilenameForRoot(slug), artifact.TypeReviewFlowBuild, rfBuild); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, sequenceModelFilenameForRoot(slug), artifact.TypeSequenceModel, diagram); err != nil {
		return nil, err
	} else {
		refs = append(refs, ref)
	}
	if mermaidCode != "" {
		if ref, err := w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, mermaidFilenameForRoot(req, slug), artifact.TypeMermaidDiagram, mermaidCode); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	if req.IncludeCandidates && rfBuild != nil {
		candidateRefs, err := w.saveCandidateRenderArtifacts(req, slug, *rfBuild, diagram.Title)
		if err != nil {
			return nil, err
		}
		refs = append(refs, candidateRefs...)
	}
	return refs, nil
}

func (w Workflow) saveCandidateRenderArtifacts(req Request, slug string, build reviewflow_build.BuildResult, title string) ([]artifact.Ref, error) {
	var refs []artifact.Ref
	for _, candidate := range build.Candidates {
		if candidate.ID == build.SelectedID {
			continue
		}
		diagram, err := w.sequenceModel.BuildFromReviewFlow(candidate, sequence_model_build.Options{Title: title})
		if err != nil {
			return nil, err
		}
		mermaidCode, err := w.mermaidEmit.Emit(diagram)
		if err != nil {
			return nil, err
		}
		sequenceFilename := candidateSequenceModelFilename(slug, candidate.Metadata.CandidateKind)
		diagramFilename := candidateMermaidFilename(slug, candidate.Metadata.CandidateKind)
		if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, sequenceFilename, artifact.TypeSequenceModel, diagram); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
		if ref, err := w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, diagramFilename, artifact.TypeMermaidDiagram, mermaidCode); err != nil {
			return nil, err
		} else {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

type renderedRoot struct {
	reviewFlow      *reviewflow.Flow
	reviewFlowBuild *reviewflow_build.BuildResult
	diagram         sequence.Diagram
	mermaidCode     string
}

func (w Workflow) renderRoot(req Request, snapshot graph.GraphSnapshot, root entrypoint.Root, reducedChain reduced.Chain, audit flow_stitch.SemanticAuditRoot, opts sequence_model_build.Options) (renderedRoot, RootRenderDecision, error) {
	mode := w.renderModeForRoot(req, root)
	decision := newRootRenderDecision(req, root, mode)
	if mode == RenderModeReview {
		decision.ReviewAttempted = true

		buildResult, err := w.reviewFlow.Build(snapshot, root, reducedChain, audit)
		if err != nil {
			return w.handleReviewFailure(req, reducedChain, opts, renderedRoot{}, decision, ReviewFallbackReasonReviewRenderError, fmt.Errorf("build reviewflow: %w", err))
		}

		rendered := renderedRoot{reviewFlowBuild: &buildResult}
		decision.ReviewFlowBuildPresent = true
		populateSelectedDecisionMetadata(&decision, buildResult)
		decision.ReviewSelected = buildResult.SelectedID != "" || buildResult.Selected.RootNodeID != "" || buildResult.Selected.ID != ""

		if len(buildResult.Candidates) == 0 {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonReviewBuildEmpty, nil)
		}
		if buildResult.SelectedID == "" && buildResult.Selected.RootNodeID == "" && buildResult.Selected.ID == "" {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonNoSelectedCandidate, nil)
		}
		if buildResult.SelectedID != "" && buildResult.Selected.RootNodeID == "" {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonIncompleteReviewArtifacts, nil)
		}

		selected := buildResult.Selected
		rendered.reviewFlow = &selected
		decision.ReviewFlowPresent = true

		if err := reviewflow_build.ValidateSelectedBuildResult(buildResult); err != nil {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, reviewFallbackReasonForValidation(err), err)
		}

		diagram, err := w.sequenceModel.BuildFromReviewFlow(selected, opts)
		if err != nil {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonReviewRenderError, fmt.Errorf("build review sequence: %w", err))
		}
		rendered.diagram = diagram
		decision.SequenceModelPresent = hasRenderableSequence(diagram)
		if !decision.SequenceModelPresent {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonIncompleteReviewArtifacts, nil)
		}

		mermaidCode, err := w.mermaidEmit.Emit(diagram)
		if err != nil {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonReviewRenderError, fmt.Errorf("emit review mermaid: %w", err))
		}
		rendered.mermaidCode = mermaidCode
		decision.MermaidPresent = strings.TrimSpace(mermaidCode) != ""
		if !decision.MermaidPresent {
			return w.handleReviewFailure(req, reducedChain, opts, rendered, decision, ReviewFallbackReasonIncompleteReviewArtifacts, nil)
		}

		decision.UsedRenderer = UsedRendererReviewFlow
		return rendered, decision, nil
	}

	return w.renderReducedRoot(reducedChain, opts, renderedRoot{}, decision)
}

func (w Workflow) renderModeForRoot(req Request, root entrypoint.Root) RenderMode {
	switch req.RenderMode {
	case RenderModeReview:
		return RenderModeReview
	case RenderModeReducedDebug:
		return RenderModeReducedDebug
	}
	if root.RootType == entrypoint.RootHTTP && (root.Framework != "" || root.Method != "" || root.Path != "") {
		return RenderModeReview
	}
	return RenderModeReducedDebug
}

func diagramTitle(req Request) string {
	if req.ServiceShortName != "" {
		return req.ServiceShortName + " — " + string(req.RootType) + " flow"
	}
	if req.RootSelector != "" {
		return req.RootSelector
	}
	return string(req.RootType) + " flow"
}

func diagramTitleForRoot(req Request, root entrypoint.Root) string {
	if req.ServiceShortName != "" {
		return req.ServiceShortName + " — " + root.CanonicalName
	}
	return root.CanonicalName
}

func mermaidFilename(req Request) string {
	base := "diagram"
	if req.ServiceShortName != "" {
		base = req.ServiceShortName
	}
	suffix := string(req.RootType)
	if suffix == "" {
		suffix = "master"
	}
	return base + "_" + suffix + ".mmd"
}

func mermaidFilenameForRoot(req Request, slug string) string {
	suffix := string(req.RootType)
	if suffix == "" {
		suffix = "master"
	}
	return "diagram_" + suffix + "__" + slug + ".mmd"
}

func reducedChainFilenameForRoot(slug string) string {
	return "reduced_chain__" + slug + ".json"
}

func sequenceModelFilenameForRoot(slug string) string {
	return "sequence_model__" + slug + ".json"
}

func reviewFlowFilenameForRoot(slug string) string {
	return "review_flow__" + slug + ".json"
}

func reviewFlowBuildFilenameForRoot(slug string) string {
	return "review_flow_build__" + slug + ".json"
}

func candidateSequenceModelFilename(slug, candidateKind string) string {
	base := "candidate_sequence_model__" + candidateKind
	if slug != "" {
		base += "__" + slug
	}
	return base + ".json"
}

func candidateMermaidFilename(slug, candidateKind string) string {
	base := "candidate_diagram__" + candidateKind
	if slug != "" {
		base += "__" + slug
	}
	return base + ".mmd"
}

func filterRoots(full entrypoint.Result, req Request) entrypoint.Result {
	var kind entrypoint.RootType
	filterByType := true
	switch req.RootType {
	case RootFilterBootstrap:
		kind = entrypoint.RootBootstrap
	case RootFilterHTTP:
		kind = entrypoint.RootHTTP
	case RootFilterWorker:
		kind = entrypoint.RootWorker
	case RootFilterMaster, "":
		filterByType = false
	default:
		filterByType = false
	}

	var filtered []entrypoint.Root
	for _, root := range full.Roots {
		matchesType := !filterByType || root.RootType == kind
		matchesSelector := req.RootSelector == "" || root.CanonicalName == req.RootSelector || root.NodeID == req.RootSelector
		if matchesType && matchesSelector {
			filtered = append(filtered, root)
		}
	}
	return entrypoint.Result{Roots: filtered}
}

type debugBundle struct {
	dir                 string
	boundaryRoots       []boundaryroot.Root
	boundaryDiagnostics []symbol.Diagnostic
	resolvedRoots       entrypoint.Result
	flowBundle          *flow.Bundle
	boundaryBundle      *boundary.Bundle
	rootExports         []RootExport
	rootRenderDecisions []RootRenderDecision
	semanticAudit       *flow_stitch.SemanticAudit
	reducedChain        *reduced.Chain
	reviewFlow          *reviewflow.Flow
	reviewFlowBuild     *reviewflow_build.BuildResult
	sequenceModel       *sequence.Diagram
	mermaidCode         string
	renderDecision      *RootRenderDecision
	rootRenderOutputs   []rootRenderDebug
	rootSemanticAudits  []rootSemanticAuditDebug
}

type rootRenderDebug struct {
	Slug            string
	ReducedChain    *reduced.Chain
	ReviewFlow      *reviewflow.Flow
	ReviewFlowBuild *reviewflow_build.BuildResult
	Sequence        *sequence.Diagram
	MermaidCode     string
	RenderDecision  *RootRenderDecision
}

type rootSemanticAuditDebug struct {
	Slug  string
	Audit flow_stitch.SemanticAuditRoot
}

func (d debugBundle) write() error {
	if d.dir == "" {
		return nil
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}

	roots := d.boundaryRoots
	if roots == nil {
		roots = []boundaryroot.Root{}
	}
	if err := saveDebugJSON(filepath.Join(d.dir, "boundary_roots.json"), roots); err != nil {
		return err
	}

	diagnostics := d.boundaryDiagnostics
	if diagnostics == nil {
		diagnostics = []symbol.Diagnostic{}
	}
	if err := saveDebugJSON(filepath.Join(d.dir, "boundary_diagnostics.json"), diagnostics); err != nil {
		return err
	}

	resolvedRoots := d.resolvedRoots
	if resolvedRoots.Roots == nil {
		resolvedRoots.Roots = []entrypoint.Root{}
	}
	if err := saveDebugJSON(filepath.Join(d.dir, "resolved_roots.json"), resolvedRoots); err != nil {
		return err
	}

	if d.flowBundle != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "flow_bundle.json"), d.flowBundle); err != nil {
			return err
		}
	}
	if d.boundaryBundle != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "boundary_bundle.json"), d.boundaryBundle); err != nil {
			return err
		}
	}
	if d.semanticAudit != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "semantic_audit.json"), d.semanticAudit); err != nil {
			return err
		}
	}

	rootExports := d.rootExports
	if rootExports == nil {
		rootExports = []RootExport{}
	}
	if err := saveDebugJSON(filepath.Join(d.dir, "root_exports.json"), rootExports); err != nil {
		return err
	}

	rootRenderDecisions := d.rootRenderDecisions
	if rootRenderDecisions == nil {
		rootRenderDecisions = []RootRenderDecision{}
	}
	if err := saveDebugJSON(filepath.Join(d.dir, "root_render_decisions.json"), rootRenderDecisions); err != nil {
		return err
	}
	if d.renderDecision != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "render_decision.json"), d.renderDecision); err != nil {
			return err
		}
	}

	if d.reducedChain != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "reduced_chain.json"), d.reducedChain); err != nil {
			return err
		}
	}
	if d.reviewFlow != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "review_flow.json"), d.reviewFlow); err != nil {
			return err
		}
	}
	if d.reviewFlowBuild != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "review_flow_build.json"), d.reviewFlowBuild); err != nil {
			return err
		}
	}
	if d.sequenceModel != nil {
		if err := saveDebugJSON(filepath.Join(d.dir, "sequence_model.json"), d.sequenceModel); err != nil {
			return err
		}
	}
	if d.mermaidCode != "" {
		if err := os.WriteFile(filepath.Join(d.dir, "diagram.mmd"), []byte(d.mermaidCode), 0o644); err != nil {
			return err
		}
	}

	for _, rootDebug := range d.rootRenderOutputs {
		if rootDebug.Slug == "" {
			continue
		}
		rootDir := filepath.Join(d.dir, "roots", rootDebug.Slug)
		if err := os.MkdirAll(rootDir, 0o755); err != nil {
			return err
		}
		if rootDebug.ReducedChain != nil {
			if err := saveDebugJSON(filepath.Join(rootDir, "reduced_chain.json"), rootDebug.ReducedChain); err != nil {
				return err
			}
		}
		if rootDebug.ReviewFlow != nil {
			if err := saveDebugJSON(filepath.Join(rootDir, "review_flow.json"), rootDebug.ReviewFlow); err != nil {
				return err
			}
		}
		if rootDebug.ReviewFlowBuild != nil {
			if err := saveDebugJSON(filepath.Join(rootDir, "review_flow_build.json"), rootDebug.ReviewFlowBuild); err != nil {
				return err
			}
		}
		if rootDebug.Sequence != nil {
			if err := saveDebugJSON(filepath.Join(rootDir, "sequence_model.json"), rootDebug.Sequence); err != nil {
				return err
			}
		}
		if rootDebug.RenderDecision != nil {
			if err := saveDebugJSON(filepath.Join(rootDir, "render_decision.json"), rootDebug.RenderDecision); err != nil {
				return err
			}
		}
		if rootDebug.MermaidCode != "" {
			if err := os.WriteFile(filepath.Join(rootDir, "diagram.mmd"), []byte(rootDebug.MermaidCode), 0o644); err != nil {
				return err
			}
		}
	}
	for _, auditDebug := range d.rootSemanticAudits {
		if auditDebug.Slug == "" {
			continue
		}
		rootDir := filepath.Join(d.dir, "roots", auditDebug.Slug)
		if err := os.MkdirAll(rootDir, 0o755); err != nil {
			return err
		}
		if err := saveDebugJSON(filepath.Join(rootDir, "semantic_audit.json"), auditDebug.Audit); err != nil {
			return err
		}
	}

	return nil
}

func ensureNonEmptyRoots(result entrypoint.Result, rootType RootTypeFilter) error {
	if len(result.Roots) > 0 {
		return nil
	}
	return fmt.Errorf("no %s roots remained after entrypoint resolution", effectiveRootType(rootType))
}

func ensureNonEmptyChains(bundle flow.Bundle) error {
	if len(bundle.Chains) > 0 {
		return nil
	}
	return fmt.Errorf("no rooted execution chains were stitched from the resolved roots")
}

func ensureNonEmptyReducedChain(chain reduced.Chain) error {
	if chain.RootNodeID != "" {
		return nil
	}
	return fmt.Errorf("reduced chain is empty because no rooted chain survived reduction")
}

func ensureNonEmptySequence(diagram sequence.Diagram) error {
	if len(diagram.Participants) != 0 || len(diagram.Elements) != 0 {
		return nil
	}
	return fmt.Errorf("sequence model is empty because no participants or elements were produced")
}

func effectiveRootType(rootType RootTypeFilter) string {
	if rootType == "" {
		return string(RootFilterMaster)
	}
	return string(rootType)
}

func saveDebugJSON(path string, data any) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

func buildFlowMetrics(roots entrypoint.Result, bundle flow.Bundle, links boundary.Bundle, rootExports []RootExport) quality.FlowQualityMetrics {
	stitchedEdges := 0
	for _, chain := range bundle.Chains {
		stitchedEdges += len(chain.Steps)
	}

	confirmed, subset, candidate, mismatch, externalOnly := 0, 0, 0, 0, 0
	for _, link := range links.Links {
		switch link.Status {
		case boundary.StatusConfirmed:
			confirmed++
		case boundary.StatusCompatibleSubset:
			subset++
		case boundary.StatusCandidate:
			candidate++
		case boundary.StatusMismatch:
			mismatch++
		case boundary.StatusExternalOnly:
			externalOnly++
		}
	}

	rendered := renderedRootCount(rootExports)
	return quality.FlowQualityMetrics{
		ResolvedEntrypoints:     len(roots.Roots),
		StitchedEdges:           stitchedEdges,
		BoundaryMarkers:         len(bundle.BoundaryMarkers),
		ConfirmedLinks:          confirmed,
		SubsetCompatibleLinks:   subset,
		CandidateLinks:          candidate,
		MismatchLinks:           mismatch,
		ExternalOnlyLinks:       externalOnly,
		ReducedChainsGenerated:  rendered,
		MermaidExportsGenerated: rendered,
	}
}

func usesPerRootHTTPExports(req Request, roots entrypoint.Result) bool {
	return req.RootType == RootFilterHTTP && req.RootSelector == "" && len(roots.Roots) > 1
}

func mapChainsByRoot(chains []flow.Chain) map[string]flow.Chain {
	result := make(map[string]flow.Chain, len(chains))
	for _, chain := range chains {
		result[chain.RootNodeID] = chain
	}
	return result
}

func selectedRoot(result entrypoint.Result, nodeID string) entrypoint.Root {
	for _, root := range result.Roots {
		if root.NodeID == nodeID {
			return root
		}
	}
	return entrypoint.Root{}
}

func renderedRootCount(rootExports []RootExport) int {
	count := 0
	for _, rootExport := range rootExports {
		if rootExport.Status == RootExportRendered {
			count++
		}
	}
	return count
}

func manifestArtifactRefs(workspaceID string, refs []artifact.Ref) []artifact.Ref {
	manifestRefs := make([]artifact.Ref, 0, len(refs))
	for _, ref := range refs {
		manifestRefs = append(manifestRefs, artifact.Ref{
			Type:        ref.Type,
			WorkspaceID: workspaceID,
			Path:        filepath.Base(ref.Path),
		})
	}
	return manifestRefs
}

func semanticAuditRootByNodeID(report *flow_stitch.SemanticAudit, nodeID string) (flow_stitch.SemanticAuditRoot, bool) {
	if report == nil {
		return flow_stitch.SemanticAuditRoot{}, false
	}
	for _, root := range report.Roots {
		if root.RootNodeID == nodeID {
			return root, true
		}
	}
	return flow_stitch.SemanticAuditRoot{}, false
}

type rootRenderOutput struct {
	exportIndex     int
	reducedChain    reduced.Chain
	reviewFlow      *reviewflow.Flow
	reviewFlowBuild *reviewflow_build.BuildResult
	sequence        sequence.Diagram
	mermaidCode     string
	renderDecision  RootRenderDecision
}

func (w Workflow) handleReviewFailure(req Request, reducedChain reduced.Chain, opts sequence_model_build.Options, rendered renderedRoot, decision RootRenderDecision, reason ReviewFallbackReason, cause error) (renderedRoot, RootRenderDecision, error) {
	decision.FallbackReason = reason
	if req.ReviewStrict {
		return rendered, decision, strictReviewError(reason, cause)
	}
	decision.FallbackUsed = true
	return w.renderReducedRoot(reducedChain, opts, renderedRoot{}, decision)
}

func (w Workflow) renderReducedRoot(reducedChain reduced.Chain, opts sequence_model_build.Options, rendered renderedRoot, decision RootRenderDecision) (renderedRoot, RootRenderDecision, error) {
	diagram, err := w.sequenceModel.Build(reducedChain, opts)
	if err != nil {
		return rendered, decision, err
	}
	rendered.diagram = diagram
	decision.SequenceModelPresent = hasRenderableSequence(diagram)

	mermaidCode, err := w.mermaidEmit.Emit(diagram)
	if err != nil {
		return rendered, decision, err
	}
	rendered.mermaidCode = mermaidCode
	decision.MermaidPresent = strings.TrimSpace(mermaidCode) != ""
	decision.UsedRenderer = UsedRendererReducedChain
	return rendered, decision, nil
}

func newRootRenderDecision(req Request, root entrypoint.Root, effectiveMode RenderMode) RootRenderDecision {
	return RootRenderDecision{
		RootNodeID:    root.NodeID,
		CanonicalName: root.CanonicalName,
		Slug:          rootExportSlug(root),
		RequestedMode: requestedRenderMode(req),
		EffectiveMode: effectiveMode,
	}
}

func requestedRenderMode(req Request) RenderMode {
	if req.RenderMode == "" {
		return RenderModeAuto
	}
	return req.RenderMode
}

func populateSelectedDecisionMetadata(decision *RootRenderDecision, build reviewflow_build.BuildResult) {
	decision.CandidateCount = len(build.Candidates)
	decision.SelectedCandidateID = build.SelectedID
	decision.SelectedSignature = build.Signature
	if build.Selected.Metadata.CandidateKind != "" {
		decision.SelectedCandidateKind = build.Selected.Metadata.CandidateKind
	}
	if decision.SelectedCandidateKind == "" {
		for _, candidate := range build.Candidates {
			if candidate.ID == build.SelectedID {
				decision.SelectedCandidateKind = candidate.Metadata.CandidateKind
				if decision.SelectedSignature == "" {
					decision.SelectedSignature = candidate.Metadata.Signature
				}
				break
			}
		}
	}
}

func reviewFallbackReasonForValidation(err error) ReviewFallbackReason {
	switch {
	case errors.Is(err, reviewflow_build.ErrBuildEmpty):
		return ReviewFallbackReasonReviewBuildEmpty
	case errors.Is(err, reviewflow_build.ErrNoSelectedCandidate):
		return ReviewFallbackReasonNoSelectedCandidate
	default:
		return ReviewFallbackReasonReviewValidationFailed
	}
}

func strictReviewError(reason ReviewFallbackReason, cause error) error {
	if cause == nil {
		return fmt.Errorf("review strict mode rejected review rendering: %s", reason)
	}
	return fmt.Errorf("review strict mode rejected review rendering: %s: %w", reason, cause)
}

func hasRenderableSequence(diagram sequence.Diagram) bool {
	return len(diagram.Participants) != 0 || len(diagram.Elements) != 0
}

func debugSequenceDiagram(diagram sequence.Diagram) *sequence.Diagram {
	if diagram.Title == "" && len(diagram.Participants) == 0 && len(diagram.Elements) == 0 {
		return nil
	}
	clone := diagram
	return &clone
}

func rootExportSlug(root entrypoint.Root) string {
	base := slugify(root.CanonicalName)
	if base == "" {
		base = "root"
	}
	suffix := stableIDSuffix(root.NodeID)
	return base + "__" + suffix
}

func stableIDSuffix(id string) string {
	if id == "" {
		return "unknown"
	}
	if idx := strings.LastIndex(id, "_"); idx >= 0 && idx+1 < len(id) {
		id = id[idx+1:]
	}
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	return strings.ToLower(id)
}

func slugify(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return slug
}
