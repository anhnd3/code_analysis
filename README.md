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

### Current Go Semantic Support

Phase 1 / PR1 currently supports bounded Go semantic extraction for:
- `RETURNS_HANDLER` from direct returned function literals
- `SPAWNS` from direct goroutine calls and inline goroutine closures
- `DEFERS` from direct deferred calls and inline deferred closures
- `WAITS_ON` from zero-argument `wg.Wait()`

Current intentional deferrals:
- alias-chain resolution
- wrapper unrolling
- channel topology inference
- branch/control shaping beyond the existing compile-safe path
- reducer shaping and final flow generation

The operating policy for this work is documented in [AGENTS.md](AGENTS.md) and the PR1 scope doc at [docs/master_plan/phase1_pr1_semantic_core.md](docs/master_plan/phase1_pr1_semantic_core.md).

Expected commands:

- `analysis-cli analyze-workspace`
- `analysis-cli build-snapshot`
- `analysis-cli blast-radius`
- `analysis-cli impacted-tests`
- `analysis-cli export-mermaid`
- `analysis-cli build-all-mermaid`

### Mermaid Flow Export

Generate Mermaid sequence diagrams from your Go codebase.

```bash
# Analyze current workspace and export bootstrap flows
analysis-cli export-mermaid --workspace . --root-type bootstrap

# Export from a previously generated snapshot JSON
analysis-cli export-mermaid --snapshot ./artifacts/snapshot.json --root-type http

# Generate a debug bundle for flow analysis
analysis-cli export-mermaid --workspace . --emit-debug-bundle --debug-out ./debug-flow/

# Fail loudly if review rendering cannot produce a valid review output
analysis-cli export-mermaid --workspace . --root-type http --render-mode review --review-strict
```

#### Debug Bundles
When `--emit-debug-bundle` is used, the following files are produced in the output directory:
- `boundary_roots.json`: Discovered framework entrypoints (Gin, net/http, etc.)
- `flow_bundle.json`: Raw stitched call chains
- `reduced_chain.json`: Chains after control-flow reduction and helper collapsing
- `review_flow.json`: Selected reviewflow when review rendering wins
- `review_flow_build.json`: Candidate set and deterministic selection metadata
- `sequence_model.json`: The final ordered participant/message model
- `diagram.mmd`: The Mermaid source code
- `root_render_decisions.json`: Root-level render path decisions for every rendered root
- `render_decision.json`: Single-root render decision details
- `roots/<slug>/render_decision.json`: Per-root render decision details for multi-root HTTP exports

`--review-strict` makes review-mode runs fail instead of silently falling back to reduced rendering.

Render decisions use:
- `used_renderer`: `reviewflow` or `reduced_chain`
- `fallback_reason`: `no_selected_candidate`, `incomplete_review_artifacts`, `review_validation_failed`, `review_build_empty`, or `review_render_error`

#### Tips for Multi-Framework Services
- Use `--service-name <name>` to label the main participant in the diagram.
- If the diagram is too complex, try `--collapse-mode aggressive`.
- Use `--root-selector <canonical_name>` to focus on a specific endpoint or function.

#### Service-Pack Confirmation
For Slice 2 and Slice 3 confirmation runs, use the service-pack runner:

```bash
bash ./scripts/run_execution_c_service_pack.sh
```

That script runs `export-mermaid` in `service_pack` mode for the ZPA and Scan audit workspaces, using explicit expected-root manifests and isolated artifact/database roots.

The per-service output directories include:
- `service_coverage_report.json`
- `service_review_pack.json`
- `selected_flows.json`
- `root_exports.json`
- `flows/*.mmd`
- `flows/*__review_flow.json`
- `flows/*__sequence_model.json`

Service-pack HTTP reviewflows now use a policy-aware entry abstraction so the diagrams read as `Client -> framework -> Handler` instead of exposing the route endpoint as a top-level participant.

The selected-flow manifest now includes selection metadata used for review:
- `candidate_kind`
- `signature`
- `participant_count`
- `stage_count`
- `message_count`
- `policy_family`
- `policy_source`
- `render_source`
- `quality_flags`

`service_review_pack.md` also includes a per-flow quality checklist with a review verdict so you can tell which flows are intentionally deferred to Slice 4.

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
