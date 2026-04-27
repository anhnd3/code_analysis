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
	"analysis-module/internal/workflows/analyze_workspace"
	"analysis-module/internal/workflows/facts_index"
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
		"Notes:",
		"  scan is the primary alias for workspace discovery.",
		"  export-mermaid is primary only with --review flag.",
	}
	for _, line := range lines {
		fmt.Fprintln(os.Stderr, line)
	}
}

func runScan(app *bootstrap.Application, args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	workspacePath := fs.String("workspace", ".", "workspace path")
	ignore := fs.String("ignore", "", "comma separated ignore patterns")
	progressMode := fs.String("progress-mode", "auto", "progress mode: auto|tty|plain|quiet")
	_ = fs.Parse(args)
	app = rebuildApp(app, *progressMode)
	result, err := app.Scan.Run(analyze_workspace.Request{
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
