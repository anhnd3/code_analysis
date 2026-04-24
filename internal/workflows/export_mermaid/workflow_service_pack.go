package export_mermaid

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/boundaryroot"
	"analysis-module/internal/domain/entrypoint"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewpack"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/sequence_model_build"
	"analysis-module/internal/services/service_review_pack"
)

type serviceCoverageReport struct {
	ServiceName string                    `json:"service_name"`
	Coverage    []reviewpack.CoverageItem `json:"coverage"`
}

func (w Workflow) runServicePackExport(req Request, snapshot graph.GraphSnapshot, inventory repository.Inventory, detectedRoots []boundaryroot.Root, filtered entrypoint.Result, debug debugBundle) (Result, error) {
	if strings.TrimSpace(req.ExpectedRootsFile) == "" {
		return Result{}, fmt.Errorf("export_mermaid: expected_roots_file is required when review_scope=service_pack")
	}

	packSvc := service_review_pack.New()
	expectedRoots, err := packSvc.LoadExpectedRoots(req.ExpectedRootsFile)
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: load expected roots: %w", err)
	}

	bundle, err := w.flowStitch.Build(snapshot, filtered, inventory)
	debug.flowBundle = &bundle
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: stitch flows: %w", err)
	}
	semanticAudit := w.flowStitch.BuildAudit(snapshot, filtered, bundle)
	debug.semanticAudit = &semanticAudit

	links, err := w.crossBoundaryLink.Build(snapshot, inventory, bundle)
	debug.boundaryBundle = &links
	if err != nil {
		_ = debug.write()
		return Result{}, fmt.Errorf("export_mermaid: link boundaries: %w", err)
	}

	outcomes := map[string]service_review_pack.RenderOutcome{}
	chainByRoot := mapChainsByRoot(bundle.Chains)
	artifactRefs := make([]artifact.Ref, 0, 8)

	if ref, saveErr := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "flow_bundle.json", artifact.TypeFlowBundle, bundle); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, saveErr := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "boundary_bundle.json", artifact.TypeBoundaryBundle, links); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}

	for _, expected := range expectedRoots {
		resolvedRoot, hasResolved := matchExpectedRootForRender(expected, filtered.Roots)
		if !hasResolved {
			continue
		}
		outcome := service_review_pack.RenderOutcome{
			ExpectedRootID: expected.ID,
			RootNodeID:     resolvedRoot.NodeID,
			CanonicalName:  resolvedRoot.CanonicalName,
			ArtifactSlug:   expected.ID,
			Family:         expected.Family,
		}
		if strings.TrimSpace(expected.Family) != "" {
			outcome.PolicySource = reviewpack.PolicySourceManifest
		} else {
			outcome.PolicySource = reviewpack.PolicySourceDefault
		}

		if strings.TrimSpace(expected.RootType) != string(entrypoint.RootHTTP) {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonUnsupportedRootType
			outcome.FailureStage = reviewpack.FailureStageSelection
			outcomes[expected.ID] = outcome
			continue
		}

		chain, hasChain := chainByRoot[resolvedRoot.NodeID]
		if !hasChain {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonNoStitchedChain
			outcome.FailureStage = reviewpack.FailureStageStitching
			outcomes[expected.ID] = outcome
			continue
		}

		reducedChain, reduceErr := w.chainReduce.ReduceChain(snapshot, chain, bundle.BoundaryMarkers, links, chain_reduce.Request{
			MaxDepth:     req.MaxDepth,
			MaxBranches:  req.MaxBranches,
			CollapseMode: req.CollapseMode,
		})
		if reduceErr != nil {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonReductionEmpty
			outcome.FailureStage = reviewpack.FailureStageReduction
			outcomes[expected.ID] = outcome
			continue
		}
		if reducedChain.RootNodeID == "" {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonReductionEmpty
			outcome.FailureStage = reviewpack.FailureStageReduction
			outcomes[expected.ID] = outcome
			continue
		}

		auditRoot, _ := semanticAuditRootByNodeID(&semanticAudit, resolvedRoot.NodeID)
		rendered, decision, renderErr := w.renderRoot(req, snapshot, resolvedRoot, reducedChain, auditRoot, sequence_model_build.Options{
			Title:            diagramTitleForRoot(req, resolvedRoot),
			ServiceShortName: req.ServiceShortName,
		})
		debug.rootRenderDecisions = append(debug.rootRenderDecisions, decision)
		if renderErr != nil {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonReviewRenderFailed
			outcome.FailureStage = reviewpack.FailureStageRendering
			outcomes[expected.ID] = outcome
			continue
		}
		if !hasRenderableSequence(rendered.diagram) || strings.TrimSpace(rendered.mermaidCode) == "" {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonReviewRenderFailed
			outcome.FailureStage = reviewpack.FailureStageRendering
			outcomes[expected.ID] = outcome
			continue
		}

		rootRefs, saveErr := w.saveServicePackFlowArtifacts(req, expected.ID, rendered)
		if saveErr != nil {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonReviewRenderFailed
			outcome.FailureStage = reviewpack.FailureStageRendering
			outcomes[expected.ID] = outcome
			continue
		}
		artifactRefs = append(artifactRefs, rootRefs...)

		outcome.Status = reviewpack.CoverageRendered
		outcome.RenderSource = renderSourceFromDecision(decision)
		outcome.MermaidPath = filepath.ToSlash(filepath.Join("flows", expected.ID+".mmd"))
		outcome.ReviewFlowPath = filepath.ToSlash(filepath.Join("flows", expected.ID+"__review_flow.json"))
		outcome.SequenceModelPath = filepath.ToSlash(filepath.Join("flows", expected.ID+"__sequence_model.json"))
		outcomes[expected.ID] = outcome
	}

	pack, err := packSvc.Build(service_review_pack.BuildInput{
		ServiceName:        servicePackServiceName(req, filtered),
		ExpectedRoots:      expectedRoots,
		ResolvedRoots:      filtered.Roots,
		DetectedBoundaries: detectedRoots,
		Outcomes:           outcomes,
	})
	if err != nil {
		return Result{}, fmt.Errorf("export_mermaid: build service review pack: %w", err)
	}
	if err := validateCoverageInvariants(pack.Coverage); err != nil {
		return Result{}, fmt.Errorf("export_mermaid: invalid service coverage: %w", err)
	}

	if ref, saveErr := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "service_coverage_report.json", artifact.TypeServiceCoverageReport, serviceCoverageReport{
		ServiceName: pack.ServiceName,
		Coverage:    pack.Coverage,
	}); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, saveErr := w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, "service_coverage_report.md", artifact.TypeServiceCoverageReport, service_review_pack.CoverageMarkdown(pack)); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, saveErr := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "selected_flows.json", artifact.TypeSelectedFlows, pack.SelectedFlows); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, saveErr := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "service_review_pack.json", artifact.TypeServiceReviewPack, pack); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	if ref, saveErr := w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, "service_review_pack.md", artifact.TypeServiceReviewPack, service_review_pack.Markdown(pack)); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}

	rootExports := rootExportsFromCoverage(pack.Coverage, expectedRoots, outcomes)
	if ref, saveErr := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, "root_exports.json", artifact.TypeRootExports, rootExports); saveErr == nil {
		artifactRefs = append(artifactRefs, ref)
	}
	debug.rootExports = rootExports
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

func (w Workflow) saveServicePackFlowArtifacts(req Request, expectedRootID string, rendered renderedRoot) ([]artifact.Ref, error) {
	refs := make([]artifact.Ref, 0, 3)
	sequencePath := filepath.ToSlash(filepath.Join("flows", expectedRootID+"__sequence_model.json"))
	reviewFlowPath := filepath.ToSlash(filepath.Join("flows", expectedRootID+"__review_flow.json"))
	mermaidPath := filepath.ToSlash(filepath.Join("flows", expectedRootID+".mmd"))

	if rendered.reviewFlow != nil {
		ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, reviewFlowPath, artifact.TypeReviewFlow, rendered.reviewFlow)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	ref, err := w.artifactStore.SaveJSON(req.WorkspaceID, req.SnapshotID, sequencePath, artifact.TypeSequenceModel, rendered.diagram)
	if err != nil {
		return nil, err
	}
	refs = append(refs, ref)
	ref, err = w.artifactStore.SaveText(req.WorkspaceID, req.SnapshotID, mermaidPath, artifact.TypeMermaidDiagram, rendered.mermaidCode)
	if err != nil {
		return nil, err
	}
	refs = append(refs, ref)
	return refs, nil
}

func matchExpectedRootForRender(expected reviewpack.ExpectedRoot, roots []entrypoint.Root) (entrypoint.Root, bool) {
	rootType := strings.TrimSpace(expected.RootType)
	method := strings.ToUpper(strings.TrimSpace(expected.Method))
	path := strings.TrimSpace(expected.Path)

	if rootType == string(entrypoint.RootBootstrap) {
		bootstrapRoots := make([]entrypoint.Root, 0, len(roots))
		for _, root := range roots {
			if root.RootType == entrypoint.RootBootstrap {
				bootstrapRoots = append(bootstrapRoots, root)
			}
		}
		if len(bootstrapRoots) == 0 {
			return entrypoint.Root{}, false
		}
		sort.SliceStable(bootstrapRoots, func(i, j int) bool {
			return bootstrapRoots[i].CanonicalName < bootstrapRoots[j].CanonicalName
		})
		return bootstrapRoots[0], true
	}

	for _, root := range roots {
		if string(root.RootType) != rootType {
			continue
		}
		if strings.EqualFold(root.Method, method) && root.Path == path {
			return root, true
		}
	}
	return entrypoint.Root{}, false
}

func renderSourceFromDecision(decision RootRenderDecision) reviewpack.RenderSource {
	if decision.UsedRenderer == UsedRendererReducedChain {
		return reviewpack.RenderSourceReducedDebug
	}
	return reviewpack.RenderSourceReviewFlow
}

func servicePackServiceName(req Request, filtered entrypoint.Result) string {
	if strings.TrimSpace(req.ServiceShortName) != "" {
		return req.ServiceShortName
	}
	if len(filtered.Roots) == 0 {
		return "service_pack"
	}
	serviceID := strings.TrimSpace(filtered.Roots[0].ServiceID)
	if serviceID != "" {
		return serviceID
	}
	repoID := strings.TrimSpace(filtered.Roots[0].RepositoryID)
	if repoID != "" {
		return repoID
	}
	return "service_pack"
}

func rootExportsFromCoverage(coverage []reviewpack.CoverageItem, expected []reviewpack.ExpectedRoot, outcomes map[string]service_review_pack.RenderOutcome) []RootExport {
	expectedByID := make(map[string]reviewpack.ExpectedRoot, len(expected))
	for _, item := range expected {
		expectedByID[item.ID] = item
	}

	result := make([]RootExport, 0, len(coverage))
	for _, item := range coverage {
		exp := expectedByID[item.ExpectedRootID]
		canonical := strings.TrimSpace(strings.TrimSpace(exp.Method) + " " + strings.TrimSpace(exp.Path))
		if canonical == "" {
			canonical = item.ExpectedRootID
		}
		if outcome, ok := outcomes[item.ExpectedRootID]; ok && strings.TrimSpace(outcome.CanonicalName) != "" {
			canonical = outcome.CanonicalName
		}

		export := RootExport{
			RootNodeID:    item.RootNodeID,
			CanonicalName: canonical,
			Slug:          item.ExpectedRootID,
			Reason:        string(item.Reason),
			Status:        RootExportSkipped,
		}
		if item.Status == reviewpack.CoverageRendered {
			export.Status = RootExportRendered
		}
		result = append(result, export)
	}
	return result
}

func validateCoverageInvariants(items []reviewpack.CoverageItem) error {
	for _, item := range items {
		if item.Status == reviewpack.CoverageRendered {
			if item.FailureStage != "" {
				return fmt.Errorf("%s is rendered but has failure_stage=%s", item.ExpectedRootID, item.FailureStage)
			}
			if item.Reason != "" {
				return fmt.Errorf("%s is rendered but has reason=%s", item.ExpectedRootID, item.Reason)
			}
			continue
		}
		if item.FailureStage == "" {
			return fmt.Errorf("%s is not rendered but has empty failure_stage", item.ExpectedRootID)
		}
		if item.Reason == "" {
			return fmt.Errorf("%s is not rendered but has empty reason", item.ExpectedRootID)
		}
	}
	return nil
}
