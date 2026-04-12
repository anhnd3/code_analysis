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
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/blast_radius"
	"analysis-module/internal/workflows/build_review_bundle"
	"analysis-module/internal/workflows/build_snapshot"
	"analysis-module/internal/workflows/impacted_tests"
)

func main() {
	logger := logging.New()
	app, err := bootstrap.New(config.Default(), logger)
	if err != nil {
		fatal(err)
	}
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("expected subcommand: analyze-workspace | build-snapshot | build-review-bundle | blast-radius | impacted-tests"))
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
