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
	"analysis-module/internal/domain/reviewgraph"
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_review_bundle"
	"analysis-module/internal/workflows/build_snapshot"
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
		fatal(fmt.Errorf("expected subcommand: analyze-workspace | build-snapshot | build-review-bundle | blast-radius | impacted-tests | graph"))
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
	mode := fs.String("mode", "", "selection mode: manual|entrypoints")
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
	includeAsync := fs.Bool("include-async", true, "include async traversal")
	forwardDepth := fs.Int("forward-depth", 2, "bounded forward depth")
	reverseDepth := fs.Int("reverse-depth", 2, "bounded reverse depth")
	outDir := fs.String("out", "", "review directory output path")
	_ = fs.Parse(args)
	result, err := app.ReviewGraphExport.Run(review_graph_export.Request{
		DBPath:       *dbPath,
		TargetsFile:  *targetsFile,
		Mode:         *mode,
		IncludeAsync: *includeAsync,
		ForwardDepth: *forwardDepth,
		ReverseDepth: *reverseDepth,
		OutDir:       *outDir,
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
