# Architecture Overview (post Phase 4)

## Package Layout

```text
cmd/analysis-cli          — CLI entry point
internal/app              — Bootstrap, config, lifecycle, logging
internal/indexer          — Workspace scanning & symbol extraction
internal/facts            — Fact model types & builders
internal/flow             — Flow skeleton (types, trace, resolver, verifier) — Phase 4 root package
internal/review           — LLM-led review service (transitional; superseded by internal/flow in Phase 4)
internal/export           — Export services: markdown.go, mermaid.go, graphjson.go
internal/llm              — LLM integration for analysis
```

## Primary Flow

```text
scan -> index -> inspect-function -> review-flow -> export-md / export-mermaid / export-graphjson
```

## Export Services

`internal/export` provides three renderers:

| Service | Output format | Description |
|---------|---------------|-------------|
| `MarkdownService` | Markdown text | Human-readable flow review with accepted steps, uncertainty notes |
| `MermaidService` | Mermaid sequence diagram | Visual call-graph rendering |
| `GraphJSONService` | JSON `{ nodes[], edges[] }` | Deduped graph suitable for Cytoscape, Graphviz, etc. |

## Cleanup Guards

- `scripts/check_no_legacy_refs.sh` — Scans `cmd/`, `internal/`, and `README.md` for blocked terms (old nested packages, legacy commands).
- `scripts/test_required_baseline.sh` — Runs the required test baseline across all active packages after checking legacy refs.

## Final Gate

```bash
gofmt -w ./cmd ./internal
./scripts/check_no_legacy_refs.sh
./scripts/test_required_baseline.sh
go test -mod=readonly ./cmd/... ./internal/...
```

All four steps must succeed before merging any change.