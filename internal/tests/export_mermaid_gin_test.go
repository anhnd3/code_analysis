package tests

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/reviewflow"
	"analysis-module/internal/domain/sequence"
	"analysis-module/internal/services/reviewflow_build"
	"analysis-module/internal/tests/fixtures"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
)

// We check both the flag (if defined in this package) and the env var.
var updateGoldens = flag.Bool("update-goldens", false, "update golden files")

func TestExportMermaidGin(t *testing.T) {
	fixturesList := []string{
		"gin_route_direct",
		"gin_route_closure",
		"gin_route_async",
		"gin_processor_branch",
		"nethttp_route_direct",
		"nethttp_route_factory",
		"grpc_gateway_registration",
		"zpa_camera_config_be",
	}

	for _, fixtureName := range fixturesList {
		t.Run(fixtureName, func(t *testing.T) {
			runGinExportTest(t, fixtureName)
		})
	}
}

type fixtureExportRun struct {
	result    export_mermaid.Result
	artifacts map[string][]byte
	debugDir  string
}

func runGinExportTest(t *testing.T, fixtureName string) {
	cfg := config.Default()
	cfg.ArtifactRoot = t.TempDir()
	cfg.SQLitePath = filepath.Join(cfg.ArtifactRoot, "analysis.sqlite")
	app, err := bootstrap.New(cfg, logging.New())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	workspacePath := fixtures.WorkspacePath(t, fixtureName)
	snapshotResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath: workspacePath,
	})
	if err != nil {
		t.Fatalf("build snapshot for %s: %v", fixtureName, err)
	}

	reviewReq := export_mermaid.Request{
		WorkspaceID:  snapshotResult.WorkspaceID,
		SnapshotID:   snapshotResult.Snapshot.ID,
		RootType:     export_mermaid.RootFilterHTTP,
		RenderMode:   export_mermaid.RenderModeReview,
		ReviewStrict: true,
	}

	first := runFixtureExport(t, app, snapshotResult, reviewReq)
	assertGoldenArtifacts(t, fixtureName, first)
	assertReviewArtifacts(t, first)

	second := runFixtureExport(t, app, snapshotResult, reviewReq)
	assertReviewRunStable(t, first, second)

	if comparisonFixture(fixtureName) {
		reduced := runFixtureExport(t, app, snapshotResult, export_mermaid.Request{
			WorkspaceID: snapshotResult.WorkspaceID,
			SnapshotID:  snapshotResult.Snapshot.ID,
			RootType:    export_mermaid.RootFilterHTTP,
			RenderMode:  export_mermaid.RenderModeReducedDebug,
		})
		assertReviewVsReduced(t, fixtureName, first, reduced)
	}
}

func runFixtureExport(t *testing.T, app *bootstrap.Application, snapshotResult build_snapshot.Result, req export_mermaid.Request) fixtureExportRun {
	t.Helper()

	req.DebugBundleDir = t.TempDir()
	result, err := app.ExportMermaid.Run(req, snapshotResult.Inventory, snapshotResult.Snapshot)
	if err != nil {
		t.Fatalf("export mermaid for %s: %v", req.RootType, err)
	}

	artifacts := map[string][]byte{}
	for _, ref := range result.ArtifactRefs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatalf("read artifact %s: %v", ref.Path, err)
		}
		artifacts[filepath.Base(ref.Path)] = data
	}

	return fixtureExportRun{
		result:    result,
		artifacts: artifacts,
		debugDir:  req.DebugBundleDir,
	}
}

func assertGoldenArtifacts(t *testing.T, fixtureName string, run fixtureExportRun) {
	t.Helper()

	goldenBase := filepath.Join("testdata", "golden", "gin_export", fixtureName)
	for _, ref := range run.result.ArtifactRefs {
		actual := run.artifacts[filepath.Base(ref.Path)]
		goldenPath := filepath.Join(goldenBase, filepath.Base(ref.Path))
		assertGoldenContent(t, goldenPath, actual)
	}
}

func assertReviewArtifacts(t *testing.T, run fixtureExportRun) {
	t.Helper()

	build := mustReviewBuild(t, run)
	flow := mustReviewFlow(t, run)
	diagram := mustSequenceDiagram(t, run)
	mermaid := mustArtifactText(t, run, "diagram_http.mmd")

	if build.SelectedID == "" || build.Signature == "" {
		t.Fatalf("expected selected review metadata, got %+v", build)
	}
	if flow.ID != build.SelectedID {
		t.Fatalf("expected review_flow id %q to match selected id %q", flow.ID, build.SelectedID)
	}

	assertNoVisibleReviewLeaksInFlow(t, flow)
	assertNoVisibleReviewLeaksInSequence(t, diagram)
	assertNoVisibleReviewLeaksInText(t, mermaid)

	decision := mustRenderDecision(t, run)
	if decision.UsedRenderer != export_mermaid.UsedRendererReviewFlow || decision.FallbackUsed {
		t.Fatalf("expected strict review fixture export to use reviewflow, got %+v", decision)
	}
	if !decision.ReviewFlowPresent || !decision.ReviewFlowBuildPresent || !decision.SequenceModelPresent || !decision.MermaidPresent {
		t.Fatalf("expected review artifacts to be present, got %+v", decision)
	}
}

func assertReviewRunStable(t *testing.T, first fixtureExportRun, second fixtureExportRun) {
	t.Helper()

	firstBuild := mustReviewBuild(t, first)
	secondBuild := mustReviewBuild(t, second)
	if firstBuild.SelectedID != secondBuild.SelectedID {
		t.Fatalf("expected stable selected id, got %q vs %q", firstBuild.SelectedID, secondBuild.SelectedID)
	}
	if firstBuild.Signature != secondBuild.Signature {
		t.Fatalf("expected stable selected signature, got %q vs %q", firstBuild.Signature, secondBuild.Signature)
	}
	if firstBuild.Selected.Metadata.CandidateKind != secondBuild.Selected.Metadata.CandidateKind {
		t.Fatalf("expected stable selected candidate kind, got %q vs %q", firstBuild.Selected.Metadata.CandidateKind, secondBuild.Selected.Metadata.CandidateKind)
	}

	firstFlow := mustReviewFlow(t, first)
	secondFlow := mustReviewFlow(t, second)
	if !slices.Equal(participantLabels(firstFlow), participantLabels(secondFlow)) {
		t.Fatalf("expected stable participant labels, got %v vs %v", participantLabels(firstFlow), participantLabels(secondFlow))
	}
	if !slices.Equal(stageLabels(firstFlow), stageLabels(secondFlow)) {
		t.Fatalf("expected stable stage labels, got %v vs %v", stageLabels(firstFlow), stageLabels(secondFlow))
	}

	firstMermaid := mustArtifactText(t, first, "diagram_http.mmd")
	secondMermaid := mustArtifactText(t, second, "diagram_http.mmd")
	if firstMermaid != secondMermaid {
		t.Fatalf("expected stable selected Mermaid output, got:\n%s\n---\n%s", firstMermaid, secondMermaid)
	}
}

func assertReviewVsReduced(t *testing.T, fixtureName string, reviewRun fixtureExportRun, reducedRun fixtureExportRun) {
	t.Helper()

	if len(reviewRun.result.RootExports) != 1 || len(reducedRun.result.RootExports) != 1 {
		t.Fatalf("expected single-root comparison fixtures, got %+v vs %+v", reviewRun.result.RootExports, reducedRun.result.RootExports)
	}
	if reviewRun.result.RootExports[0].CanonicalName != reducedRun.result.RootExports[0].CanonicalName {
		t.Fatalf("expected same root identity, got %q vs %q", reviewRun.result.RootExports[0].CanonicalName, reducedRun.result.RootExports[0].CanonicalName)
	}

	reviewSemanticAudit := mustReadFile(t, filepath.Join(reviewRun.debugDir, "semantic_audit.json"))
	reducedSemanticAudit := mustReadFile(t, filepath.Join(reducedRun.debugDir, "semantic_audit.json"))
	if !slices.Equal(reviewSemanticAudit, reducedSemanticAudit) {
		t.Fatalf("expected same semantic audit baseline for %s", fixtureName)
	}
	if !slices.Equal(mustArtifactData(t, reviewRun, "reduced_chain.json"), mustArtifactData(t, reducedRun, "reduced_chain.json")) {
		t.Fatalf("expected same reduced-chain input for %s", fixtureName)
	}

	reviewFlow := mustReviewFlow(t, reviewRun)
	reviewDiagram := mustSequenceDiagram(t, reviewRun)
	reducedDiagram := mustSequenceDiagram(t, reducedRun)
	reviewDecision := mustRenderDecision(t, reviewRun)
	reducedDecision := mustRenderDecision(t, reducedRun)

	if reviewDecision.UsedRenderer != export_mermaid.UsedRendererReviewFlow {
		t.Fatalf("expected review run to use reviewflow, got %+v", reviewDecision)
	}
	if reducedDecision.UsedRenderer != export_mermaid.UsedRendererReducedChain {
		t.Fatalf("expected reduced run to use reduced_chain, got %+v", reducedDecision)
	}
	if len(reviewDiagram.Participants) > len(reducedDiagram.Participants) {
		t.Fatalf("expected review diagram to be no noisier than reduced diagram for %s", fixtureName)
	}
	if countStageNotes(reviewDiagram) == 0 {
		t.Fatalf("expected review diagram to contain stage notes for %s", fixtureName)
	}
	if len(reviewFlow.Blocks) > 0 && !sequenceHasBlock(reviewDiagram) {
		t.Fatalf("expected review diagram to preserve block evidence for %s", fixtureName)
	}
	if len(reviewFlow.Notes) > 0 && countNonStageNotes(reviewDiagram) == 0 {
		t.Fatalf("expected review diagram to preserve note evidence for %s", fixtureName)
	}
}

func comparisonFixture(name string) bool {
	switch name {
	case "gin_route_closure", "gin_route_async", "gin_processor_branch", "zpa_camera_config_be":
		return true
	default:
		return false
	}
}

func mustReviewBuild(t *testing.T, run fixtureExportRun) reviewflow_build.BuildResult {
	t.Helper()

	var build reviewflow_build.BuildResult
	if err := json.Unmarshal(mustArtifactData(t, run, "review_flow_build.json"), &build); err != nil {
		t.Fatalf("unmarshal review_flow_build.json: %v", err)
	}
	return build
}

func mustReviewFlow(t *testing.T, run fixtureExportRun) reviewflow.Flow {
	t.Helper()

	var flow reviewflow.Flow
	if err := json.Unmarshal(mustArtifactData(t, run, "review_flow.json"), &flow); err != nil {
		t.Fatalf("unmarshal review_flow.json: %v", err)
	}
	return flow
}

func mustSequenceDiagram(t *testing.T, run fixtureExportRun) sequence.Diagram {
	t.Helper()

	var diagram sequence.Diagram
	if err := json.Unmarshal(mustArtifactData(t, run, "sequence_model.json"), &diagram); err != nil {
		t.Fatalf("unmarshal sequence_model.json: %v", err)
	}
	return diagram
}

func mustRenderDecision(t *testing.T, run fixtureExportRun) export_mermaid.RootRenderDecision {
	t.Helper()

	data := mustReadFile(t, filepath.Join(run.debugDir, "render_decision.json"))
	var decision export_mermaid.RootRenderDecision
	if err := json.Unmarshal(data, &decision); err != nil {
		t.Fatalf("unmarshal render_decision.json: %v", err)
	}
	return decision
}

func mustArtifactData(t *testing.T, run fixtureExportRun, name string) []byte {
	t.Helper()

	data, ok := run.artifacts[name]
	if !ok {
		t.Fatalf("expected artifact %s, got %v", name, sortedKeys(run.artifacts))
	}
	return data
}

func mustArtifactText(t *testing.T, run fixtureExportRun, name string) string {
	t.Helper()
	return string(mustArtifactData(t, run, name))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func participantLabels(flow reviewflow.Flow) []string {
	labels := make([]string, 0, len(flow.Participants))
	for _, participant := range flow.Participants {
		labels = append(labels, participant.Label)
	}
	return labels
}

func stageLabels(flow reviewflow.Flow) []string {
	labels := make([]string, 0, len(flow.Stages))
	for _, stage := range flow.Stages {
		labels = append(labels, stage.Kind+":"+stage.Label)
	}
	return labels
}

func assertNoVisibleReviewLeaksInFlow(t *testing.T, flow reviewflow.Flow) {
	t.Helper()

	for _, participant := range flow.Participants {
		if hasVisibleReviewLeak(participant.Label) {
			t.Fatalf("unexpected visible artifact in participant %+v", participant)
		}
	}
	for _, stage := range flow.Stages {
		if hasVisibleReviewLeak(stage.Label) {
			t.Fatalf("unexpected visible artifact in stage %+v", stage)
		}
		for _, message := range stage.Messages {
			if hasVisibleReviewLeak(message.Label) {
				t.Fatalf("unexpected visible artifact in message %+v", message)
			}
		}
	}
	for _, block := range flow.Blocks {
		if hasVisibleReviewLeak(block.Label) {
			t.Fatalf("unexpected visible artifact in block %+v", block)
		}
		for _, section := range block.Sections {
			if hasVisibleReviewLeak(section.Label) {
				t.Fatalf("unexpected visible artifact in block section %+v", section)
			}
			for _, message := range section.Messages {
				if hasVisibleReviewLeak(message.Label) {
					t.Fatalf("unexpected visible artifact in block message %+v", message)
				}
			}
		}
	}
	for _, note := range flow.Notes {
		if hasVisibleReviewLeak(note.Text) {
			t.Fatalf("unexpected visible artifact in note %+v", note)
		}
	}
}

func assertNoVisibleReviewLeaksInSequence(t *testing.T, diagram sequence.Diagram) {
	t.Helper()

	for _, participant := range diagram.Participants {
		if hasVisibleReviewLeak(participant.Label) {
			t.Fatalf("unexpected visible artifact in sequence participant %+v", participant)
		}
	}
	for _, element := range diagram.Elements {
		switch {
		case element.Message != nil:
			if hasVisibleReviewLeak(element.Message.Label) {
				t.Fatalf("unexpected visible artifact in sequence message %+v", element.Message)
			}
		case element.Note != nil:
			if hasVisibleReviewLeak(element.Note.Text) {
				t.Fatalf("unexpected visible artifact in sequence note %+v", element.Note)
			}
		case element.Block != nil:
			if hasVisibleReviewLeak(element.Block.Label) {
				t.Fatalf("unexpected visible artifact in sequence block %+v", element.Block)
			}
			for _, section := range element.Block.Sections {
				if hasVisibleReviewLeak(section.Label) {
					t.Fatalf("unexpected visible artifact in sequence block section %+v", section)
				}
				for _, message := range section.Messages {
					if hasVisibleReviewLeak(message.Label) {
						t.Fatalf("unexpected visible artifact in sequence block message %+v", message)
					}
				}
			}
		}
	}
}

func assertNoVisibleReviewLeaksInText(t *testing.T, text string) {
	t.Helper()

	lower := strings.ToLower(text)
	for _, token := range []string{"$closure_", "$inline_", "go func", "func()", "wg.", "defer ", "<-"} {
		if strings.Contains(lower, token) {
			t.Fatalf("unexpected visible artifact token %q in text:\n%s", token, text)
		}
	}
}

func hasVisibleReviewLeak(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "$closure") || strings.Contains(lower, "$inline") {
		return true
	}
	for _, token := range []string{"go func", "func()", "wg.", "defer ", "<-"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func countStageNotes(diagram sequence.Diagram) int {
	count := 0
	for _, element := range diagram.Elements {
		if element.Note != nil && strings.HasPrefix(element.Note.Text, "Stage: ") {
			count++
		}
	}
	return count
}

func countNonStageNotes(diagram sequence.Diagram) int {
	count := 0
	for _, element := range diagram.Elements {
		if element.Note != nil && !strings.HasPrefix(element.Note.Text, "Stage: ") {
			count++
		}
	}
	return count
}

func sequenceHasBlock(diagram sequence.Diagram) bool {
	for _, element := range diagram.Elements {
		if element.Block != nil {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func assertGoldenContent(t *testing.T, path string, actual []byte) {
	t.Helper()
	update := *updateGoldens || os.Getenv("UPDATE_GOLDEN") == "1"

	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		if update {
			return // already written
		}
		t.Fatalf("read golden file %s: %v (run with UPDATE_GOLDEN=1 to initialize)", path, err)
	}

	if !slices.Equal(actual, expected) {
		t.Errorf("golden mismatch for %s", path)
	}
}
