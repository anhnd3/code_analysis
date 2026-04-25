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
	"analysis-module/internal/domain/reduced"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/reviewpack"
	"analysis-module/internal/services/chain_reduce"
	"analysis-module/internal/services/flow_stitch"
	"analysis-module/internal/services/reviewflow_bootstrap"
	"analysis-module/internal/services/reviewflow_build"
	"analysis-module/internal/services/reviewflow_expand"
	"analysis-module/internal/services/reviewflow_policy"
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
	policySvc := reviewflow_policy.New()
	bootstrapSvc := reviewflow_bootstrap.New()
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
			PolicyFamily:   expected.Family,
		}
		if strings.TrimSpace(expected.Family) != "" {
			outcome.PolicySource = reviewpack.PolicySourceManifest
		} else {
			outcome.PolicySource = reviewpack.PolicySourceDefault
		}

		if strings.TrimSpace(expected.RootType) == string(entrypoint.RootBootstrap) {
			bootstrapFlow, bootstrapErr := bootstrapSvc.Build(snapshot, resolvedRoot)
			if bootstrapErr != nil {
				outcome.Status = reviewpack.CoverageSkipped
				outcome.Reason = reviewpack.ReasonBootstrapInsufficient
				outcome.FailureStage = reviewpack.FailureStageRendering
				outcomes[expected.ID] = outcome
				continue
			}
			diagram, seqErr := w.sequenceModel.BuildFromReviewFlow(bootstrapFlow, sequence_model_build.Options{
				Title:            diagramTitleForRoot(req, resolvedRoot),
				ServiceShortName: req.ServiceShortName,
			})
			if seqErr != nil || !hasRenderableSequence(diagram) {
				outcome.Status = reviewpack.CoverageSkipped
				outcome.Reason = reviewpack.ReasonBootstrapInsufficient
				outcome.FailureStage = reviewpack.FailureStageRendering
				outcomes[expected.ID] = outcome
				continue
			}
			mermaidCode, mermaidErr := w.mermaidEmit.Emit(diagram)
			if mermaidErr != nil || strings.TrimSpace(mermaidCode) == "" {
				outcome.Status = reviewpack.CoverageSkipped
				outcome.Reason = reviewpack.ReasonBootstrapInsufficient
				outcome.FailureStage = reviewpack.FailureStageRendering
				outcomes[expected.ID] = outcome
				continue
			}
			rootRefs, saveErr := w.saveServicePackFlowArtifacts(req, expected.ID, renderedRoot{
				reviewFlow:  &bootstrapFlow,
				diagram:     diagram,
				mermaidCode: mermaidCode,
			})
			if saveErr != nil {
				outcome.Status = reviewpack.CoverageSkipped
				outcome.Reason = reviewpack.ReasonBootstrapInsufficient
				outcome.FailureStage = reviewpack.FailureStageRendering
				outcomes[expected.ID] = outcome
				continue
			}
			artifactRefs = append(artifactRefs, rootRefs...)

			outcome.Status = reviewpack.CoverageRendered
			outcome.RenderSource = reviewpack.RenderSourceBootstrapLifecycle
			outcome.CandidateKind = bootstrapFlow.Metadata.CandidateKind
			outcome.Signature = bootstrapFlow.Metadata.Signature
			outcome.ParticipantCount = len(bootstrapFlow.Participants)
			outcome.StageCount = len(bootstrapFlow.Stages)
			outcome.MessageCount = reviewFlowMessageCount(bootstrapFlow)
			outcome.QualityFlags = qualityFlagsForRenderedFlow(policyForExpectedBootstrap(expected), &bootstrapFlow, mermaidCode, outcome.RenderSource)
			outcome.MermaidPath = filepath.ToSlash(filepath.Join("flows", expected.ID+".mmd"))
			outcome.ReviewFlowPath = filepath.ToSlash(filepath.Join("flows", expected.ID+"__review_flow.json"))
			outcome.SequenceModelPath = filepath.ToSlash(filepath.Join("flows", expected.ID+"__sequence_model.json"))
			outcomes[expected.ID] = outcome
			continue
		}

		if strings.TrimSpace(expected.RootType) != string(entrypoint.RootHTTP) {
			outcome.Status = reviewpack.CoverageSkipped
			outcome.Reason = reviewpack.ReasonUnsupportedRootType
			outcome.FailureStage = reviewpack.FailureStageSelection
			outcomes[expected.ID] = outcome
			continue
		}
		policyResult := policySvc.Resolve(reviewflow_policy.ResolveInput{
			Root:           resolvedRoot,
			ExpectedFamily: expected.Family,
			ServiceName:    servicePackServiceName(req, filtered),
			WorkspacePath:  req.WorkspaceID,
		})
		outcome.PolicySource = policyResult.Source
		outcome.Family = policyResult.Policy.Family
		outcome.PolicyFamily = policyResult.Policy.Family

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
		rendered, decision, renderErr := w.renderRootWithPolicy(req, snapshot, resolvedRoot, reducedChain, auditRoot, sequence_model_build.Options{
			Title:            diagramTitleForRoot(req, resolvedRoot),
			ServiceShortName: req.ServiceShortName,
		}, policyResult.Policy)
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
		if rendered.reviewFlow != nil {
			outcome.CandidateKind = rendered.reviewFlow.Metadata.CandidateKind
			outcome.Signature = rendered.reviewFlow.Metadata.Signature
			outcome.ParticipantCount = len(rendered.reviewFlow.Participants)
			outcome.StageCount = len(rendered.reviewFlow.Stages)
			outcome.MessageCount = reviewFlowMessageCount(*rendered.reviewFlow)
		}
		outcome.QualityFlags = qualityFlagsForRenderedFlow(policyResult.Policy, rendered.reviewFlow, rendered.mermaidCode, outcome.RenderSource)
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
		return pickBootstrapRoot(roots)
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

func pickBootstrapRoot(roots []entrypoint.Root) (entrypoint.Root, bool) {
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
		left := bootstrapRoots[i]
		right := bootstrapRoots[j]
		if bootstrapConfidenceRank(left.Confidence) != bootstrapConfidenceRank(right.Confidence) {
			return bootstrapConfidenceRank(left.Confidence) > bootstrapConfidenceRank(right.Confidence)
		}
		if bootstrapCanonicalRank(left.CanonicalName) != bootstrapCanonicalRank(right.CanonicalName) {
			return bootstrapCanonicalRank(left.CanonicalName) < bootstrapCanonicalRank(right.CanonicalName)
		}
		return left.CanonicalName < right.CanonicalName
	})
	return bootstrapRoots[0], true
}

func bootstrapConfidenceRank(confidence entrypoint.Confidence) int {
	switch confidence {
	case entrypoint.ConfidenceHigh:
		return 3
	case entrypoint.ConfidenceMedium:
		return 2
	case entrypoint.ConfidenceLow:
		return 1
	default:
		return 0
	}
}

func bootstrapCanonicalRank(canonical string) int {
	name := strings.ToLower(strings.TrimSpace(canonical))
	switch {
	case name == "main.main":
		return 0
	case strings.HasSuffix(name, ".main"):
		return 1
	case strings.Contains(name, "main"):
		return 2
	default:
		return 3
	}
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

func (w Workflow) renderRootWithPolicy(req Request, snapshot graph.GraphSnapshot, root entrypoint.Root, reducedChain reduced.Chain, audit flow_stitch.SemanticAuditRoot, opts sequence_model_build.Options, policy reviewflow_policy.Policy) (renderedRoot, RootRenderDecision, error) {
	mode := w.renderModeForRoot(req, root)
	decision := newRootRenderDecision(req, root, mode)
	if mode == RenderModeReview {
		decision.ReviewAttempted = true

		buildResult, err := w.buildReviewFlowWithPolicy(snapshot, root, reducedChain, audit, policy)
		if err != nil {
			return w.handleReviewFailure(req, reducedChain, opts, renderedRoot{}, decision, ReviewFallbackReasonReviewRenderError, fmt.Errorf("build reviewflow with policy: %w", err))
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

func (w Workflow) buildReviewFlowWithPolicy(snapshot graph.GraphSnapshot, root entrypoint.Root, chain reduced.Chain, audit flow_stitch.SemanticAuditRoot, policy reviewflow_policy.Policy) (reviewflow_build.BuildResult, error) {
	type optionsBuilder interface {
		BuildWithOptions(snapshot graph.GraphSnapshot, root entrypoint.Root, chain reduced.Chain, audit flow_stitch.SemanticAuditRoot, options reviewflow_build.BuildOptions) (reviewflow_build.BuildResult, error)
	}
	type policyBuilder interface {
		BuildWithPolicy(snapshot graph.GraphSnapshot, root entrypoint.Root, chain reduced.Chain, audit flow_stitch.SemanticAuditRoot, policy reviewflow_policy.Policy) (reviewflow_build.BuildResult, error)
	}
	if builder, ok := w.reviewFlow.(optionsBuilder); ok {
		options := reviewflow_build.BuildOptions{
			Policy: &policy,
			// Expansion is only enabled when reviewflow is built through the
			// service-pack policy-aware path.
			ExpansionEnabled: true,
		}
		if policy.AddHTTPEntryParticipants {
			options.EntryMode = reviewflow_build.EntryModeServicePackHTTP
		}
		return builder.BuildWithOptions(snapshot, root, chain, audit, options)
	}
	if builder, ok := w.reviewFlow.(policyBuilder); ok {
		return builder.BuildWithPolicy(snapshot, root, chain, audit, policy)
	}
	return w.reviewFlow.Build(snapshot, root, chain, audit)
}

func policyForExpectedBootstrap(expected reviewpack.ExpectedRoot) reviewflow_policy.Policy {
	family := strings.TrimSpace(expected.Family)
	if family == "" {
		family = reviewflow_policy.FamilyBootstrapStartup
	}
	return reviewflow_policy.Policy{Family: family}
}

func reviewFlowMessageCount(flow reviewflow.Flow) int {
	count := 0
	for _, stage := range flow.Stages {
		count += len(stage.Messages)
	}
	for _, block := range flow.Blocks {
		for _, section := range block.Sections {
			count += len(section.Messages)
		}
	}
	return count
}

func qualityFlagsForRenderedFlow(policy reviewflow_policy.Policy, flow *reviewflow.Flow, mermaidCode string, renderSource reviewpack.RenderSource) []string {
	flags := map[string]bool{}

	if flow != nil && renderSource == reviewpack.RenderSourceReviewFlow && flow.SourceRootType == string(entrypoint.RootHTTP) && policy.AddHTTPEntryParticipants {
		if hasHTTPEntryAbstraction(*flow) {
			flags["entry_abstraction_present"] = true
		}
	}
	if policy.PreserveBranchBlocks {
		if flow != nil && hasBranchEvidence(*flow) {
			flags["branch_expected_present"] = true
		} else {
			flags["branch_expected_missing"] = true
		}
	}
	if policy.PreserveAsyncBlocks {
		if flow != nil && hasAsyncEvidence(*flow) {
			flags["async_expected_present"] = true
		} else {
			flags["async_expected_missing"] = true
		}
	}
	if policy.PreservePostProcessing {
		if flow != nil && hasPostProcessingEvidence(*flow) {
			flags["post_processing_expected_present"] = true
		} else {
			flags["post_processing_expected_missing"] = true
		}
	}
	if hasVisibleArtifactLeak(flow, mermaidCode) {
		flags["visible_artifact_leak"] = true
	} else {
		flags["no_visible_artifact_leak"] = true
	}

	out := make([]string, 0, len(flags))
	for flag := range flags {
		out = append(out, flag)
	}
	sort.Strings(out)
	return out
}

func hasHTTPEntryAbstraction(flow reviewflow.Flow) bool {
	if flow.SourceRootType != string(entrypoint.RootHTTP) {
		return false
	}
	clientID := ""
	frameworkID := ""
	handlerID := ""
	expectedFrameworkLabel := frameworkLabelForEntry(flow.Metadata.RootFramework)
	for _, participant := range flow.Participants {
		if isRouteLikeParticipantLabel(participant.Label) {
			return false
		}
		if participant.Label == "Client" {
			clientID = participant.ID
		}
		if participant.Label == expectedFrameworkLabel {
			frameworkID = participant.ID
		}
		if participant.Kind == "handler" || participant.Label == "Handler" {
			handlerID = participant.ID
		}
	}
	if clientID == "" || frameworkID == "" || handlerID == "" {
		return false
	}

	hasClientRequest := false
	hasHandlerInvoke := false
	for _, stage := range flow.Stages {
		for _, message := range stage.Messages {
			if message.FromParticipantID == clientID && message.ToParticipantID == frameworkID && isHTTPMethodPathLabel(message.Label) {
				hasClientRequest = true
			}
			if message.FromParticipantID == frameworkID && message.ToParticipantID == handlerID && strings.EqualFold(strings.TrimSpace(message.Label), "invoke handler") {
				hasHandlerInvoke = true
			}
		}
	}
	return hasClientRequest && hasHandlerInvoke
}

func frameworkLabelForEntry(framework string) string {
	switch strings.ToLower(strings.TrimSpace(framework)) {
	case "gin":
		return "Gin"
	case "net/http":
		return "net/http"
	case "grpc-gateway":
		return "Gateway Proxy"
	case "":
		return "HTTP Router"
	default:
		return strings.TrimSpace(framework)
	}
}

func isRouteLikeParticipantLabel(label string) bool {
	return isHTTPMethodPathLabel(label)
}

func isHTTPMethodPathLabel(label string) bool {
	parts := strings.SplitN(strings.TrimSpace(label), " ", 2)
	return len(parts) == 2 && isUpperMethodToken(parts[0]) && strings.HasPrefix(parts[1], "/")
}

func isUpperMethodToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func hasBranchEvidence(flow reviewflow.Flow) bool {
	for _, stage := range flow.Stages {
		for _, message := range stage.Messages {
			if message.Class == reviewflow_expand.ClassBranchBusiness || message.Class == reviewflow_expand.ClassBranchValidation {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		if block.Class == reviewflow_expand.ClassBranchBusiness || block.Class == reviewflow_expand.ClassBranchValidation {
			return true
		}
		if block.Kind == reviewflow.BlockAlt || block.Kind == reviewflow.BlockLoop || block.Kind == reviewflow.BlockPar {
			return true
		}
		for _, section := range block.Sections {
			for _, message := range section.Messages {
				if message.Class == reviewflow_expand.ClassBranchBusiness || message.Class == reviewflow_expand.ClassBranchValidation {
					return true
				}
			}
		}
	}
	return false
}

func hasAsyncEvidence(flow reviewflow.Flow) bool {
	for _, stage := range flow.Stages {
		if stage.Kind == "deferred_async" {
			return true
		}
		for _, message := range stage.Messages {
			if message.Kind == reviewflow.MessageAsync || message.Class == reviewflow_expand.ClassAsyncWorker || message.Class == reviewflow_expand.ClassDeferredSideEffect {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		if block.Class == reviewflow_expand.ClassAsyncWorker || block.Class == reviewflow_expand.ClassDeferredSideEffect {
			return true
		}
		if block.Kind == reviewflow.BlockPar {
			return true
		}
		for _, section := range block.Sections {
			for _, message := range section.Messages {
				if message.Kind == reviewflow.MessageAsync || message.Class == reviewflow_expand.ClassAsyncWorker || message.Class == reviewflow_expand.ClassDeferredSideEffect {
					return true
				}
			}
		}
	}
	return false
}

func hasPostProcessingEvidence(flow reviewflow.Flow) bool {
	for _, stage := range flow.Stages {
		if stage.Kind == "post_processing" {
			return true
		}
		for _, message := range stage.Messages {
			if message.Class == reviewflow_expand.ClassPostProcessing {
				return true
			}
		}
	}
	for _, block := range flow.Blocks {
		if block.Class == reviewflow_expand.ClassPostProcessing {
			return true
		}
		for _, section := range block.Sections {
			for _, message := range section.Messages {
				if message.Class == reviewflow_expand.ClassPostProcessing {
					return true
				}
			}
		}
	}
	return false
}

func hasVisibleArtifactLeak(flow *reviewflow.Flow, mermaidCode string) bool {
	containsLeak := func(value string) bool {
		lower := strings.ToLower(value)
		switch {
		case strings.Contains(lower, "$closure"), strings.Contains(lower, "$inline"), strings.Contains(lower, "goroutine"), strings.Contains(lower, "go func"), strings.Contains(lower, "func()"), strings.Contains(lower, "func ("), strings.Contains(lower, "wg."), strings.Contains(lower, "defer "), strings.Contains(lower, "<-"), strings.Contains(lower, "select {"), strings.Contains(lower, "chan "):
			return true
		default:
			return strings.ContainsAny(value, "{}")
		}
	}

	if flow != nil {
		for _, participant := range flow.Participants {
			if containsLeak(participant.Label) {
				return true
			}
		}
		for _, stage := range flow.Stages {
			if containsLeak(stage.Label) {
				return true
			}
			for _, message := range stage.Messages {
				if containsLeak(message.Label) {
					return true
				}
			}
		}
		for _, block := range flow.Blocks {
			if containsLeak(block.Label) {
				return true
			}
			for _, section := range block.Sections {
				if containsLeak(section.Label) {
					return true
				}
				for _, message := range section.Messages {
					if containsLeak(message.Label) {
						return true
					}
				}
			}
		}
		for _, note := range flow.Notes {
			if containsLeak(note.Text) {
				return true
			}
		}
	}

	lowerMermaid := strings.ToLower(mermaidCode)
	for _, token := range []string{"$closure_", "$inline_", "go func", "func()", "func (", "wg.", "defer ", "<-", "select {", "chan "} {
		if strings.Contains(lowerMermaid, token) {
			return true
		}
	}
	return false
}
