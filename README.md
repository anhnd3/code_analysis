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
internal/facts/query      — SQLite query helpers
internal/facts/sqlite     — SQLite persistence layer
internal/indexer          — Workspace scanning & symbol extraction
internal/indexer/boundary — Framework detection (Go: Gin, net/http, gRPC)
internal/indexer/detector — Language/tech-stack detectors
internal/indexer/extractor— Per-language extractors (Go, Python, JS, treesitter)
internal/review           — LLM-led review service (transitional; superseded by internal/flow in Phase 4)
internal/export           — Export services: markdown.go, mermaid.go, graphjson.go
internal/flow             — Flow skeleton (types, trace, resolver, verifier) — Phase 4 root package
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

Run before merging any change:

```bash
./scripts/check_no_legacy_refs.sh
./scripts/test_required_baseline.sh
go test -mod=mod ./cmd/... ./internal/...
```

## Final Gate (Phase 5)

```bash
gofmt -w ./cmd ./internal
./scripts/check_no_legacy_refs.sh
./scripts/test_required_baseline.sh
go test -mod=mod ./cmd/... ./internal/...
```

All four steps must succeed.
