# Code Analysis

Local SDLC evidence layer for AI-assisted refactoring and review.

## Primary Flow

```text
scan -> index -> inspect-function -> review-flow -> export-md/export-mermaid/export-graphjson
```

## Commands

| Command | Description |
|---------|-------------|
| `scan` | Discover repositories, services, files in a workspace |
| `index` | Extract symbols, imports, exports, call candidates; persist to SQLite/JSONL |
| `inspect-function` | Load bounded context packet for a specific function |
| `review-flow` | LLM-led review of outgoing calls from a root symbol (produces ReviewFlow) |
| `export-md` | Render a reviewed flow as Markdown |
| `export-mermaid` | Render a reviewed flow as Mermaid sequence diagram |
| `export-graphjson` | Render a reviewed flow as graph JSON (nodes + edges, deduped by symbol ID) |

## Package Layout

```text
cmd/analysis-cli          — CLI entry point
internal/app              — Application bootstrap, config, lifecycle, logging
internal/facts            — Fact model types & builders
internal/indexer          — Workspace scanning & symbol extraction
internal/review           — LLM-led review service (transitional; superseded by internal/flow in Phase 4)
```

## Artifact Layout

After a review + export run, artifacts are written to the output directory:

```text
<out_dir>/
  flow.json          # ReviewFlow JSON (from review-flow command)
  flow.md            # Markdown rendering (from export-md)
  flow.mmd           # Mermaid sequence diagram (from export-mermaid)
  graph.json         # Graph nodes + edges (from export-graphjson)
```

## LLM Configuration

The `review-flow` command uses an LLM planner for candidate edge resolution. Configure via:

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_BASE_URL` | — | Base URL of the OpenAI-compatible endpoint |
| `LLM_API_KEY` | — | API key |
| `LLM_MODEL` | `gpt-4o` | Model name for review planning |

## Required Baseline

The single command below is the authoritative gate for "is this stabilized?":

```bash
./scripts/test_required_baseline.sh
```

This script runs:
1. `gofmt -l internal/facts internal/indexer internal/review internal/app cmd/analysis-cli` — check formatting
2. `./scripts/check_no_legacy_refs.sh` — verify no forbidden legacy references
3. Compile-focused tests (`go test -run '^$'`) on core packages
4. Full package tests (`go test ./cmd/... ./internal/...`)
5. CLI smoke tests: `scan`, `index`, `inspect-function`, and export commands
6. Artifact verification: SQLite, JSON, and JSONL files exist and are non-empty

All checks must pass for the gate to succeed.

## Repair-Loop Process

When stabilizing this repository, follow this disciplined loop:

1. **Run the command first.** Always start with `./scripts/test_required_baseline.sh`
2. **Fix only the first meaningful failure class.** Do not attempt multiple repairs in parallel.
3. **Rerun the same command.** Verify your fix before proceeding.
4. **Repeat until PASS.** Stabilization is complete only when the gate passes cleanly.

> Note: `review-flow` is excluded from smoke tests because it requires LLM configuration; export smoke uses handcrafted ReviewFlow fixtures instead.