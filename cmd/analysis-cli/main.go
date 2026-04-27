package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/facts"
	factquery "analysis-module/internal/query"
	factreview "analysis-module/internal/review"
)

func main() {
	logger := logging.New()
	app, err := bootstrap.New(config.Default(), logger)
	if err != nil {
		fatal(err)
	}
	if len(os.Args) < 2 {
		printUsage()
		fatal(fmt.Errorf("expected a subcommand"))
	}
	switch os.Args[1] {
	case "scan":
		runScan(app, os.Args[2:])
	case "index":
		runIndex(app, os.Args[2:])
	case "inspect-function":
		runInspectFunction(app, os.Args[2:])
	case "review-flow":
		runReviewFlow(app, os.Args[2:])
	case "export-md":
		runExportMarkdown(app, os.Args[2:])
	case "export-mermaid":
		runExportMermaid(app, os.Args[2:])
	default:
		printUsage()
		fatal(fmt.Errorf("unknown subcommand: %s", os.Args[1]))
	}
}

func printUsage() {
	lines := []string{
		"Analysis Module CLI",
		"",
		"Primary path:",
		"  scan -> index -> inspect-function -> review-flow -> export-md/export-mermaid --review",
		"",
		"Compatibility only:",
		"  analyze-workspace",
		"  build-snapshot",
		"  build-review-bundle",
		"  blast-radius",
		"  impacted-tests",
		"  export-mermaid (workspace/snapshot/root/debug legacy modes)",
		"  build-all-mermaid",
		"  graph <subcommand>",
		"",
		"Notes:",
		"  scan is the primary alias for workspace discovery; analyze-workspace remains compatibility only.",
		"  export-mermaid is primary only with --review; all other export-mermaid entry modes are legacy compatibility.",
	}
	for _, line := range lines {
		fmt.Fprintln(os.Stderr, line)
	}
}

func runAnalyzeWorkspace(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("analyze-workspace", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)
	result, err := app.AnalyzeWorkspace.Run(analyze_workspace.Request{
		WorkspacePath:  *workspacePath,
		IgnorePatterns: splitCSV(*ignore),
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runScan(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)
	result, err := app.AnalyzeWorkspace.Run(analyze_workspace.Request{
		WorkspacePath:  *workspacePath,
		IgnorePatterns: splitCSV(*ignore),
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runIndex(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)
	result, err := app.FactsIndex.Run(facts_index.Request{
		WorkspacePath:  *workspacePath,
		IgnorePatterns: splitCSV(*ignore),
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runInspectFunction(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("inspect-function", flag.ExitOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id")
	snapshotID := fs.String("snapshot-id", "", "snapshot id")
	symbol := fs.String("symbol", "", "symbol canonical name or id")
	contextWindow := fs.Int("context-window", 8, "line window around symbol range")
	_ = fs.Parse(args)
	if *workspaceID == "" || *snapshotID == "" || *symbol == "" {
		fatal(fmt.Errorf("--workspace-id, --snapshot-id and --symbol are required"))
	}
	packet, err := app.FactsQuery.InspectFunction(factquery.InspectRequest{
		WorkspaceID:   *workspaceID,
		SnapshotID:    *snapshotID,
		Symbol:        *symbol,
		ContextWindow: *contextWindow,
	})
	if err != nil {
		fatal(err)
	}
	write(packet)
}

func runReviewFlow(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("review-flow", flag.ExitOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id")
	snapshotID := fs.String("snapshot-id", "", "snapshot id")
	symbol := fs.String("symbol", "", "symbol canonical name or id")
	maxDepth := fs.Int("max-depth", 3, "max review expansion depth")
	maxSteps := fs.Int("max-steps", 80, "max review steps")
	outDir := fs.String("out", "", "output directory for flow artifacts")
	_ = fs.Parse(args)
	if *workspaceID == "" || *snapshotID == "" || *symbol == "" {
		fatal(fmt.Errorf("--workspace-id, --snapshot-id and --symbol are required"))
	}
	dir := *outDir
	if dir == "" {
		dir = filepath.Join(app.Config.ArtifactRoot, "workspaces", *workspaceID, "snapshots", *snapshotID, "review")
	}
	result, err := app.FlowReview.Run(factreview.Request{
		WorkspaceID: *workspaceID,
		SnapshotID:  *snapshotID,
		Symbol:      *symbol,
		MaxDepth:    *maxDepth,
		MaxSteps:    *maxSteps,
		OutDir:      dir,
	})
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
	}
	flowPath := filepath.Join(dir, "flow.json")
	evidencePath := filepath.Join(dir, "evidence.json")
	uncertaintyPath := filepath.Join(dir, "uncertainty.md")
	if err := writeJSONFile(flowPath, result.Flow); err != nil {
		fatal(err)
	}
	if err := writeJSONFile(evidencePath, result.Flow.Steps); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(uncertaintyPath, []byte(strings.Join(result.Flow.UncertaintyNotes, "\n")), 0o644); err != nil {
		fatal(err)
	}
	write(map[string]any{
		"flow":           result.Flow,
		"flow_path":      flowPath,
		"evidence_path":  evidencePath,
		"uncertainty_md": uncertaintyPath,
	})
}

func runExportMarkdown(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("export-md", flag.ExitOnError)
	reviewPath := fs.String("review", "", "path to flow.json")
	outPath := fs.String("out", "", "markdown output path")
	_ = fs.Parse(args)
	if *reviewPath == "" {
		fatal(fmt.Errorf("--review is required"))
	}
	flow, err := readReviewFlow(*reviewPath)
	if err != nil {
		fatal(err)
	}
	body := app.FlowMarkdown.Render(flow)
	target := *outPath
	if target == "" {
		target = filepath.Join(filepath.Dir(*reviewPath), "flow.md")
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		fatal(err)
	}
	write(map[string]any{
		"out": target,
	})
}

func runBuildSnapshot(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("build-snapshot", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)
	result, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath:  *workspacePath,
		IgnorePatterns: splitCSV(*ignore),
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runBuildReviewBundle(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("build-review-bundle", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	outDir := fs.String("out", "", "bundle output directory")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)
	result, err := app.BuildReviewBundle.Run(build_review_bundle.Request{
		WorkspacePath:  *workspacePath,
		IgnorePatterns: splitCSV(*ignore),
		OutDir:         *outDir,
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runBlastRadius(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("blast-radius", flag.ExitOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id")
	snapshotID := fs.String("snapshot-id", "", "snapshot id")
	target := fs.String("target", "", "target canonical name or node id")
	maxDepth := fs.Int("max-depth", 3, "max traversal depth")
	_ = fs.Parse(args)
	result, err := app.BlastRadius.Run(blast_radius.Request{
		WorkspaceID: *workspaceID,
		SnapshotID:  *snapshotID,
		Target:      *target,
		MaxDepth:    *maxDepth,
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runImpactedTests(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("impacted-tests", flag.ExitOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id")
	snapshotID := fs.String("snapshot-id", "", "snapshot id")
	target := fs.String("target", "", "target canonical name or node id")
	maxDepth := fs.Int("max-depth", 3, "max traversal depth")
	_ = fs.Parse(args)
	result, err := app.ImpactedTests.Run(impacted_tests.Request{
		WorkspaceID: *workspaceID,
		SnapshotID:  *snapshotID,
		Target:      *target,
		MaxDepth:    *maxDepth,
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runExportMermaid(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("export-mermaid", flag.ExitOnError)
	reviewPath := fs.String("review", "", "flow.json path to render reviewed flow")
	outPath := fs.String("out", "", "mermaid output path")
	_ = fs.Parse(args)

	if *reviewPath == "" {
		fatal(fmt.Errorf("--review is required"))
	}

	flow, err := readReviewFlow(*reviewPath)
	if err != nil {
		fatal(err)
	}
	diagram := app.FlowMermaid.Render(flow)
	target := *outPath
	if target == "" {
		target = filepath.Join(filepath.Dir(*reviewPath), "flow.mmd")
	}
	if err := os.WriteFile(target, []byte(diagram), 0o644); err != nil {
		fatal(err)
	}
	write(map[string]any{
		"out": target,
	})
}

func runBuildAllMermaid(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("build-all-mermaid", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	maxDepth := fs.Int("max-depth", 30, "max traversal depth")
	maxBranches := fs.Int("max-branches", 5, "max branch limit")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)

	// Step 1: Build Snapshot
	app.Logger.Info("Building snapshot...")
	snapResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
		WorkspacePath:  *workspacePath,
		IgnorePatterns: splitCSV(*ignore),
	})
	if err != nil {
		fatal(fmt.Errorf("failed to build snapshot: %w", err))
	}
	app.Logger.Info("Snapshot built", "workspace_id", snapResult.WorkspaceID, "snapshot_id", snapResult.Snapshot.ID)

	var allResults []legacymermaid.Result

	// Step 2: Pass A - Bootstrap
	app.Logger.Info("Exporting 'bootstrap' flows...")
	resBoot, err := app.ExportMermaid.Run(legacymermaid.Request{
		WorkspaceID:  snapResult.WorkspaceID,
		SnapshotID:   snapResult.Snapshot.ID,
		RootType:     legacymermaid.RootFilterBootstrap,
		MaxDepth:     *maxDepth,
		MaxBranches:  *maxBranches,
		CollapseMode: "default",
	}, snapResult.Inventory, snapResult.Snapshot)
	if err != nil {
		app.Logger.Error("bootstrap export failed", "error", err)
	} else {
		allResults = append(allResults, resBoot)
	}

	// Step 3: Pass B - HTTP
	app.Logger.Info("Exporting 'http' endpoint flows...")
	resHTTP, err := app.ExportMermaid.Run(legacymermaid.Request{
		WorkspaceID:  snapResult.WorkspaceID,
		SnapshotID:   snapResult.Snapshot.ID,
		RootType:     legacymermaid.RootFilterHTTP,
		RenderMode:   legacymermaid.RenderModeReview,
		MaxDepth:     *maxDepth,
		CollapseMode: "default",
	}, snapResult.Inventory, snapResult.Snapshot)
	if err != nil {
		app.Logger.Error("http export failed", "error", err)
	} else {
		allResults = append(allResults, resHTTP)
	}

	// Step 4: Pass C - Worker
	app.Logger.Info("Exporting 'worker' flows...")
	resWorker, err := app.ExportMermaid.Run(legacymermaid.Request{
		WorkspaceID:  snapResult.WorkspaceID,
		SnapshotID:   snapResult.Snapshot.ID,
		RootType:     legacymermaid.RootFilterWorker,
		MaxDepth:     *maxDepth,
		CollapseMode: "aggressive",
	}, snapResult.Inventory, snapResult.Snapshot)
	if err != nil {
		app.Logger.Error("worker export failed", "error", err)
	} else {
		allResults = append(allResults, resWorker)
	}

	write(map[string]any{
		"snapshot": snapResult,
		"exports":  allResults,
	})
}

func runGraph(app *bootstrap.Application, args []string) {
	if len(args) == 0 {
		fatal(fmt.Errorf("expected legacy graph subcommand: import-sqlite | list-startpoints | export-markdown-review"))
	}
	switch args[0] {
	case "import-sqlite":
		runGraphImportSQLite(app, args[1:])
	case "list-startpoints":
		runGraphListStartpoints(app, args[1:])
	case "export-markdown-review":
		runGraphExportMarkdownReview(app, args[1:])
	default:
		fatal(fmt.Errorf("unknown legacy graph subcommand: %s", args[0]))
	}
}

func runGraphImportSQLite(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("graph import-sqlite", flag.ExitOnError)
	workspaceID := fs.String("workspace-id", "", "workspace id")
	snapshotID := fs.String("snapshot-id", "", "snapshot id")
	nodesPath := fs.String("nodes", "", "legacy graph nodes jsonl path")
	edgesPath := fs.String("edges", "", "legacy graph edges jsonl path")
	repoManifestPath := fs.String("repo-manifest", "", "repository manifests json path")
	serviceManifestPath := fs.String("service-manifest", "", "service manifests json path")
	qualityReportPath := fs.String("quality-report", "", "quality report json path")
	ignoreFilePath := fs.String("ignore-file", "", "optional text review ignore file")
	outDBPath := fs.String("out", "", "review graph sqlite output path")
	_ = fs.Parse(args)
	result, err := app.ReviewGraphImport.Run(legacyreviewgraph.ImportRequest{
		WorkspaceID:         *workspaceID,
		SnapshotID:          *snapshotID,
		NodesPath:           *nodesPath,
		EdgesPath:           *edgesPath,
		RepoManifestPath:    *repoManifestPath,
		ServiceManifestPath: *serviceManifestPath,
		QualityReportPath:   *qualityReportPath,
		IgnoreFilePath:      *ignoreFilePath,
		OutDBPath:           *outDBPath,
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runGraphListStartpoints(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("graph list-startpoints", flag.ExitOnError)
	dbPath := fs.String("db", "", "review graph sqlite path")
	mode := fs.String("mode", "workflow", "selection mode: manual|workflow|entrypoints")
	symbol := fs.String("symbol", "", "manual symbol selector")
	file := fs.String("file", "", "manual file selector")
	topic := fs.String("topic", "", "manual topic selector")
	outPath := fs.String("out", "", "resolved targets output file")
	_ = fs.Parse(args)
	result, err := app.ReviewGraphListStartpoints.Run(legacyreviewgraph.SelectRequest{
		DBPath:  *dbPath,
		Mode:    *mode,
		Symbol:  *symbol,
		File:    *file,
		Topic:   *topic,
		OutPath: *outPath,
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func runGraphExportMarkdownReview(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("graph export-markdown-review", flag.ExitOnError)
	dbPath := fs.String("db", "", "review graph sqlite path")
	targetsFile := fs.String("targets-file", "", "resolved targets json file")
	mode := fs.String("mode", string(legacyreviewgraph.TraversalFullFlow), "traversal mode: full-flow|bounded")
	renderMode := fs.String("render-mode", "grouped", "render mode: grouped|raw")
	companionView := fs.String("companion-view", "none", "companion view generation: none|overview|all")
	includeAsync := fs.Bool("include-async", true, "include async traversal")
	forwardDepth := fs.Int("forward-depth", 2, "bounded forward depth")
	reverseDepth := fs.Int("reverse-depth", 2, "bounded reverse depth")
	outDir := fs.String("out", "", "review directory output path")
	_ = fs.Parse(args)
	result, err := app.ReviewGraphExport.Run(legacyreviewgraph.ExportRequest{
		DBPath:        *dbPath,
		TargetsFile:   *targetsFile,
		Mode:          *mode,
		RenderMode:    *renderMode,
		CompanionView: *companionView,
		IncludeAsync:  *includeAsync,
		ForwardDepth:  *forwardDepth,
		ReverseDepth:  *reverseDepth,
		OutDir:        *outDir,
	})
	if err != nil {
		fatal(err)
	}
	write(result)
}

func rebuildApp(existing *bootstrap.Application, progressMode string) *bootstrap.Application {
	cfg := existing.Config
	cfg.ProgressMode = progressMode
	app, err := bootstrap.New(cfg, existing.Logger)
	if err != nil {
		fatal(err)
	}
	return app
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func readReviewFlow(path string) (facts.ReviewFlow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return facts.ReviewFlow{}, err
	}
	var flow facts.ReviewFlow
	if err := json.Unmarshal(data, &flow); err != nil {
		return facts.ReviewFlow{}, err
	}
	return flow, nil
}

func writeJSONFile(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func write(payload any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
