package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/repository"
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_review_bundle"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/export_mermaid"
	"analysis-module/internal/workflows/impacted_tests"
	"analysis-module/internal/workflows/review_graph_export"
	"analysis-module/internal/workflows/review_graph_import"
	"analysis-module/internal/workflows/review_graph_list_startpoints"
)

func main() {
	logger := logging.New()
	app, err := bootstrap.New(config.Default(), logger)
	if err != nil {
		fatal(err)
	}
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("expected subcommand: analyze-workspace | build-snapshot | build-review-bundle | blast-radius | impacted-tests | graph | export-mermaid | build-all-mermaid"))
	}
	switch os.Args[1] {
	case "analyze-workspace":
		runAnalyzeWorkspace(app, os.Args[2:])
	case "build-snapshot":
		runBuildSnapshot(app, os.Args[2:])
	case "build-review-bundle":
		runBuildReviewBundle(app, os.Args[2:])
	case "blast-radius":
		runBlastRadius(app, os.Args[2:])
	case "impacted-tests":
		runImpactedTests(app, os.Args[2:])
	case "export-mermaid":
		runExportMermaid(app, os.Args[2:])
	case "build-all-mermaid":
		runBuildAllMermaid(app, os.Args[2:])
	case "graph":
		runGraph(app, os.Args[2:])
	default:
		fatal(fmt.Errorf("unknown subcommand: %s", os.Args[1]))
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
	workspacePath := fs.String("workspace", "", "workspace path to analyze and export")
	snapshotFile := fs.String("snapshot", "", "JSON snapshot path to load and export")
	rootType := fs.String("root-type", "master", "root type: bootstrap|http|worker|symbol|master")
	rootSelector := fs.String("root-selector", "", "target canonical name or node id")
	reviewScope := fs.String("review-scope", "root", "review scope: root|service_pack")
	expectedRootsFile := fs.String("expected-roots-file", "", "path to expected roots JSON manifest for service_pack mode")
	renderMode := fs.String("render-mode", "auto", "render mode: auto|review|reduced_debug")
	reviewStrict := fs.Bool("review-strict", false, "fail review mode instead of silently falling back")
	maxDepth := fs.Int("max-depth", 30, "max traversal depth")
	maxBranches := fs.Int("max-branches", 5, "max branch limit")
	collapseMode := fs.String("collapse-mode", "default", "collapse mode: default|none|aggressive")
	serviceName := fs.String("service-name", "", "service short name")
	incCandidates := fs.Bool("include-candidates", false, "include candidate boundary links")
	emitDebug := fs.Bool("emit-debug-bundle", false, "emit debug bundle files")
	debugOut := fs.String("debug-out", "./analysis-debug/", "debug bundle output directory")
	_ = fs.Parse(args)

	if *workspacePath == "" && *snapshotFile == "" {
		fatal(fmt.Errorf("either --workspace or --snapshot must be provided"))
	}
	if *workspacePath != "" && *snapshotFile != "" {
		fatal(fmt.Errorf("only one of --workspace or --snapshot can be provided"))
	}

	var inventory repository.Inventory
	var snapshot graph.GraphSnapshot
	var workspaceID string

	if *workspacePath != "" {
		app.Logger.Info("Analyzing workspace...", "path", *workspacePath)
		snapResult, err := app.BuildSnapshot.Run(build_snapshot.Request{
			WorkspacePath: *workspacePath,
		})
		if err != nil {
			fatal(fmt.Errorf("failed to build snapshot: %w", err))
		}
		inventory = snapResult.Inventory
		snapshot = snapResult.Snapshot
		workspaceID = snapResult.WorkspaceID
	} else {
		app.Logger.Info("Loading snapshot file...", "path", *snapshotFile)
		data, err := os.ReadFile(*snapshotFile)
		if err != nil {
			fatal(fmt.Errorf("failed to read snapshot file: %w", err))
		}
		var snapResult build_snapshot.Result
		if err := json.Unmarshal(data, &snapResult); err != nil {
			fatal(fmt.Errorf("failed to unmarshal snapshot: %w", err))
		}
		inventory = snapResult.Inventory
		snapshot = snapResult.Snapshot
		workspaceID = snapResult.WorkspaceID
	}

	debugDir := ""
	if *emitDebug {
		debugDir = *debugOut
	}

	result, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:       workspaceID,
		SnapshotID:        snapshot.ID,
		RootType:          export_mermaid.RootTypeFilter(*rootType),
		RootSelector:      *rootSelector,
		ReviewScope:       export_mermaid.ReviewScope(*reviewScope),
		ExpectedRootsFile: *expectedRootsFile,
		RenderMode:        export_mermaid.RenderMode(*renderMode),
		ReviewStrict:      *reviewStrict,
		MaxDepth:          *maxDepth,
		MaxBranches:       *maxBranches,
		CollapseMode:      *collapseMode,
		ServiceShortName:  *serviceName,
		IncludeCandidates: *incCandidates,
		DebugBundleDir:    debugDir,
	}, inventory, snapshot)
	if err != nil {
		fatal(err)
	}
	// Print JSON result structure output
	write(result)
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

	var allResults []export_mermaid.Result

	// Step 2: Pass A - Bootstrap
	app.Logger.Info("Exporting 'bootstrap' flows...")
	resBoot, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:  snapResult.WorkspaceID,
		SnapshotID:   snapResult.Snapshot.ID,
		RootType:     export_mermaid.RootFilterBootstrap,
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
	resHTTP, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:  snapResult.WorkspaceID,
		SnapshotID:   snapResult.Snapshot.ID,
		RootType:     export_mermaid.RootFilterHTTP,
		RenderMode:   export_mermaid.RenderModeReview,
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
	resWorker, err := app.ExportMermaid.Run(export_mermaid.Request{
		WorkspaceID:  snapResult.WorkspaceID,
		SnapshotID:   snapResult.Snapshot.ID,
		RootType:     export_mermaid.RootFilterWorker,
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
		fatal(fmt.Errorf("expected graph subcommand: import-sqlite | list-startpoints | export-markdown-review"))
	}
	switch args[0] {
	case "import-sqlite":
		runGraphImportSQLite(app, args[1:])
	case "list-startpoints":
		runGraphListStartpoints(app, args[1:])
	case "export-markdown-review":
		runGraphExportMarkdownReview(app, args[1:])
	default:
		fatal(fmt.Errorf("unknown graph subcommand: %s", args[0]))
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
	result, err := app.ReviewGraphImport.Run(review_graph_import.Request{
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
	result, err := app.ReviewGraphListStartpoints.Run(review_graph_list_startpoints.Request{
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
	mode := fs.String("mode", string(reviewgraph.TraversalFullFlow), "traversal mode: full-flow|bounded")
	renderMode := fs.String("render-mode", "grouped", "render mode: grouped|raw")
	companionView := fs.String("companion-view", "none", "companion view generation: none|overview|all")
	includeAsync := fs.Bool("include-async", true, "include async traversal")
	forwardDepth := fs.Int("forward-depth", 2, "bounded forward depth")
	reverseDepth := fs.Int("reverse-depth", 2, "bounded reverse depth")
	outDir := fs.String("out", "", "review directory output path")
	_ = fs.Parse(args)
	result, err := app.ReviewGraphExport.Run(review_graph_export.Request{
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

func write(payload any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
