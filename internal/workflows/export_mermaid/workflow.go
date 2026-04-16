package export_mermaid

import (
	"fmt"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/boundary"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/flow"
	"analysis-module/internal/domain/quality"
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/symbol"
	artifactstoreport "analysis-module/internal/ports/artifactstore"
	graphstoreport "analysis-module/internal/ports/graphstore"
	"analysis-module/internal/services/boundary_detect"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/cross_boundary_link"
	"analysis-module/internal/services/entrypoint_resolve"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/mermaid_emit"
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

// Request configures the export_mermaid workflow.
type Request struct {
	WorkspaceID       string         `json:"workspace_id"`
	SnapshotID        string         `json:"snapshot_id"`
	RootType          RootTypeFilter `json:"root_type"`
	RootSelector      string         `json:"root_selector,omitempty"`
	MaxDepth          int            `json:"max_depth,omitempty"`
	MaxBranches       int            `json:"max_branches,omitempty"`
	CollapseMode      string         `json:"collapse_mode,omitempty"`
	ServiceShortName  string         `json:"service_short_name,omitempty"`
	IncludeCandidates bool           `json:"include_candidates,omitempty"`
}

// Result holds the workflow output.
type Result struct {
	WorkspaceID  string                     `json:"workspace_id"`
	SnapshotID   string                     `json:"snapshot_id"`
	ArtifactRefs []artifact.Ref             `json:"artifact_refs"`
	FlowMetrics  quality.FlowQualityMetrics `json:"flow_metrics"`
	MermaidCode  string                     `json:"mermaid_code,omitempty"`
}

// Workflow orchestrates the full Mermaid export pipeline.
type Workflow struct {
	graphStores       graphstoreport.Provider
	artifactStore     artifactstoreport.Store
	entrypointResolve entrypoint_resolve.Service
	flowStitch        flow_stitch.Service
	crossBoundaryLink cross_boundary_link.Service
	chainReduce       chain_reduce.Service
	sequenceModel     sequence_model_build.Service
	mermaidEmit       mermaid_emit.Service
	boundaryDetect    boundary_detect.Service
}

// New creates the export_mermaid workflow.
func New(
	graphStores graphstoreport.Provider,
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
		graphStores:       graphStores,
		artifactStore:     artifactStore,
		entrypointResolve: entrypointResolve,
		flowStitch:        flowStitch,
		crossBoundaryLink: crossBoundaryLink,
		chainReduce:       chainReduce,
		sequenceModel:     sequenceModel,
		mermaidEmit:       mermaidEmit,
		boundaryDetect:    boundaryDetect,
	}
}

// Run executes the complete Mermaid export pipeline:
//
//	load snapshot → resolve entrypoints → stitch flows → link boundaries
//	→ reduce chain → build sequence model → emit mermaid → save artifacts → metrics
func (w Workflow) Run(req Request, inventory repository.Inventory) (Result, error) {
	// 1. Load snapshot
	store, err := w.graphStores.ForWorkspace(req.WorkspaceID)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: open graph store: %w", err)
	}
	snapshot, err := store.GetSnapshot(req.SnapshotID)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: load snapshot %s: %w", req.SnapshotID, err)
	}

	// 2. Resolve entrypoints
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
	detectedRoots, _ := w.boundaryDetect.DetectAll(inventory, symbols)
	resolved, err := w.entrypointResolve.Resolve(snapshot, inventory, detectedRoots)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: resolve entrypoints: %w", err)
	}
	filtered := filterRoots(resolved, req)

	// 3. Stitch flows
	bundle, err := w.flowStitch.Build(snapshot, filtered, inventory)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: stitch flows: %w", err)
	}

	// 4. Link boundaries
	links, err := w.crossBoundaryLink.Build(snapshot, inventory, bundle)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: link boundaries: %w", err)
	}

	// 5. Reduce chain
	reducedChain, err := w.chainReduce.Reduce(snapshot, bundle, links, chain_reduce.Request{
		MaxDepth:     req.MaxDepth,
		MaxBranches:  req.MaxBranches,
		CollapseMode: req.CollapseMode,
	})
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: reduce chain: %w", err)
	}

	// 6. Build sequence model
	diagram, err := w.sequenceModel.Build(reducedChain, sequence_model_build.Options{
		Title:            diagramTitle(req),
		ServiceShortName: req.ServiceShortName,
	})
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: build sequence model: %w", err)
	}

	// 7. Emit Mermaid
	mermaidCode, err := w.mermaidEmit.Emit(diagram)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: emit mermaid: %w", err)
	}

	// 8. Save artifacts
	var artifactRefs []artifact.Ref

	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "flow_bundle.json", artifact.TypeFlowBundle, bundle); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "boundary_bundle.json", artifact.TypeBoundaryBundle, links); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "reduced_chain.json", artifact.TypeReducedChain, reducedChain); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "sequence_model.json", artifact.TypeSequenceModel, diagram); err == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if mermaidCode != "" {
		filename := mermaidFilename(req)
		if ref, err := w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, filename, artifact.TypeMermaidDiagram, mermaidCode); err == nil {
			artifactRefs = append(artifactRefs, ref)
		}
	}

	// 9. Build metrics
	metrics := buildFlowMetrics(filtered, bundle, links, reducedChain, mermaidCode)

	return Result{
		WorkspaceID:  req.WorkspaceID,
		SnapshotID:   req.SnapshotID,
		ArtifactRefs: artifactRefs,
		FlowMetrics:  metrics,
		MermaidCode:  mermaidCode,
	}, nil
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

func filterRoots(full entrypoint.Result, req Request) entrypoint.Result {
	if req.RootType == RootFilterMaster || req.RootType == "" {
		return full
	}

	var kind entrypoint.RootType
	switch req.RootType {
	case RootFilterBootstrap:
		kind = entrypoint.RootBootstrap
	case RootFilterHTTP:
		kind = entrypoint.RootHTTP
	case RootFilterWorker:
		kind = entrypoint.RootWorker
	}

	var filtered []entrypoint.Root
	for _, r := range full.Roots {
		if r.RootType == kind {
			filtered = append(filtered, r)
		} else if req.RootSelector != "" && r.CanonicalName == req.RootSelector {
			filtered = append(filtered, r)
		}
	}

	return entrypoint.Result{Roots: filtered}
}

func buildFlowMetrics(roots entrypoint.Result, bundle flow.Bundle, links boundary.Bundle, chain reduced.Chain, mermaidCode string) quality.FlowQualityMetrics {
	stitchedEdges := 0
	for _, c := range bundle.Chains {
		stitchedEdges += len(c.Steps)
	}

	confirmed, subset, candidate, mismatch, externalOnly := 0, 0, 0, 0, 0
	for _, l := range links.Links {
		switch l.Status {
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

	reducedChains := 0
	if chain.RootNodeID != "" {
		reducedChains = 1
	}

	mermaidGenerated := 0
	if mermaidCode != "" {
		mermaidGenerated = 1
	}

	return quality.FlowQualityMetrics{
		ResolvedEntrypoints:    len(roots.Roots),
		StitchedEdges:          stitchedEdges,
		BoundaryMarkers:        len(bundle.BoundaryMarkers),
		ConfirmedLinks:         confirmed,
		SubsetCompatibleLinks:  subset,
		CandidateLinks:         candidate,
		MismatchLinks:          mismatch,
		ExternalOnlyLinks:      externalOnly,
		ReducedChainsGenerated: reducedChains,
		MermaidExportsGenerated: mermaidGenerated,
	}
}
