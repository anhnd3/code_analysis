# Analysis Module

CLI-first analysis service for the local SDLC refactoring agent.

Implemented in this pass:

- workspace scanning and repository discovery
- repository inventory normalization
- Go-only Tree-Sitter-backed symbol extraction
- local static call graph construction
- SQLite metadata persistence
- JSONL graph artifacts
- blast-radius and impacted-tests queries
- thin HTTP adapter via `analysisd`

Deferred behind stable package boundaries:

- REST/gRPC/Kafka boundary stitching
- repo-map generation
- packet building
- semantic LLM harness
- advanced cache and enrichment layers

Expected commands:

- `analysis-cli analyze-workspace`
- `analysis-cli build-snapshot`
- `analysis-cli blast-radius`
- `analysis-cli impacted-tests`

The parser layer now uses the official Tree-Sitter Go bindings from `github.com/tree-sitter/go-tree-sitter` plus the official Go grammar package from `github.com/tree-sitter/tree-sitter-go/bindings/go`.

`vendor/` is intentionally stale in this pass. Until a later dependency-sync run refreshes vendoring, use module mode explicitly:

- `go mod tidy`
- `go test -mod=mod ./...`

The official bindings require explicit `Close()` calls on C-backed parser and tree objects. The parser adapter closes parsers per parse call, and extractor code closes returned trees after use.

For a richer test report with executed test cases plus coverage artifacts, run:

- `./scripts/test_report.sh`

That writes a report bundle under `artifacts/test_reports/<timestamp>/` with:

- `summary.md`
- `test-cases.tsv`
- `coverage.out`
- `module-coverage.tsv`
- `file-coverage.tsv`
- `coverage.html`

Go does not expose per-test-case coverage directly in its standard tooling. This report pairs test-case execution results from `go test -json` with statement coverage aggregated into overall, module, and file tables.
