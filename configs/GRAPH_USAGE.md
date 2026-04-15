# Review Graph Usage

## Purpose

Use the review graph when the question is:

- if this function changes, what runtime flow is affected
- which callers and callees matter in the reachable slice
- which async bridges connect the code to queues, topics, schedulers, workers, or in-process concurrency
- which files and services a reviewer should inspect next

The graph stays function-first for traversal. Review readability is improved at export time by grouping output as:

```text
file/module -> class/type -> function
```

That means repeated prefixes appear once in the rendered flow tree instead of being repeated across many flat path lines.

## Preferred One-Command Run

From the `analysis_module` directory:

```bash
bash ./scripts/run-review-report.sh /mnt/d/Workspace/Local_Agent/software_agent_src/analysis_module/
```

What the script does:

1. runs `build-snapshot --progress-mode plain`
2. runs `graph import-sqlite`
3. runs `graph list-startpoints`
4. runs `graph export-markdown-review --render-mode grouped --companion-view none --include-async`
5. runs `scripts/test_report.sh`

Environment variables supported by the script:

- `STARTPOINT_MODE`
- `RENDER_MODE`
- `COMPANION_VIEW`
- `GO_BIN`
- `TEST_REPORT_OUT`

Defaults:

- `STARTPOINT_MODE=workflow`
- `RENDER_MODE=grouped`
- `COMPANION_VIEW=none`
- `GO_BIN=go`
- `TEST_REPORT_OUT=<snapshot review dir>/test-report`

## Manual CLI Flow

### 1. Build snapshot

```bash
go run ./cmd/analysis-cli build-snapshot --workspace /path/to/workspace --progress-mode plain
```

### 2. Import review graph

```bash
go run ./cmd/analysis-cli graph import-sqlite \
  --workspace-id <workspace_id> \
  --snapshot-id <snapshot_id>
```

Optional overrides:

- `--nodes`
- `--edges`
- `--repo-manifest`
- `--service-manifest`
- `--quality-report`
- `--ignore-file`
- `--out`

### 3. Resolve startpoints

Broad review pass:

```bash
go run ./cmd/analysis-cli graph list-startpoints \
  --db artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/sqlite/review_graph.sqlite \
  --mode workflow
```

Targeted review:

```bash
go run ./cmd/analysis-cli graph list-startpoints \
  --db artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/sqlite/review_graph.sqlite \
  --mode manual \
  --symbol 'package.Service.Handle'
```

Manual selectors supported:

- `--symbol`
- `--file`
- `--topic`

### 4. Export markdown review

```bash
go run ./cmd/analysis-cli graph export-markdown-review \
  --db artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/sqlite/review_graph.sqlite \
  --targets-file artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/review/resolved_targets.json \
  --mode full-flow \
  --render-mode grouped \
  --companion-view none \
  --include-async
```

Render modes:

- `grouped`
- `raw`

Companion view modes:

- `none`
- `overview`
- `all`

Use `grouped` for normal review. Use `raw` only when you want the older flat path listing for debugging.

Traversal modes:

- `full-flow`
- `bounded`

In `bounded` mode, `--forward-depth` and `--reverse-depth` limit sync expansion.

## Startpoint Modes

### `entrypoints`

This mode creates review targets from production-facing anchors such as:

- service entrypoints
- functions and methods promoted to async producer or async consumer
- HTTP and gRPC boundaries
- scheduler jobs
- explicit topic, queue, and pubsub bridge nodes

Entry-point ordering is biased toward real functions/methods first so the default review set starts from human-meaningful runtime roots before heuristic bridge nodes.

### `manual`

This mode is for targeted impact analysis when you already know the anchor you care about.

Use it when you want to inspect:

- a specific function or method
- the most review-worthy symbol in one file
- a specific async topic, queue, channel, worker handoff, or scheduler name

## Grouped Rendering

Grouped rendering is presentation-only.

Traversal still uses the base function graph:

- sync flow follows `CALLS`
- async flow follows async bridge edges
- structural edges stay contextual only

The grouped exporter merges shared prefixes so a flow like:

```text
repo_a:main.go -> repo_b:getabc() -> repo_c:getdcb()
               -> repo_d:dosomething()
               -> repo_e:log_something()
```

renders as a tree with the shared prefix shown once.

Markers such as `root`, `entrypoint`, `leaf`, `shared_path`, `cycle`, and `truncated` are preserved on grouped branches.

## Async V2 Coverage

Async V2 keeps the older broker/scheduler bridge model and extends it to native concurrency.

Currently covered:

- Kafka and generic event/message/topic patterns
- RabbitMQ or AMQP-style queue patterns
- Redis pubsub or streams when literal/config-resolved
- NATS subjects
- SQS and SNS when literal/config-resolved
- Celery and BullMQ queue names when literal/config-resolved
- scheduler or cron-style jobs
- Go goroutines and named channel handoff
- Python `asyncio.create_task`, `ensure_future`, thread/executor submit patterns, and queue handoff
- JS/TS workers and explicit message handoff

Intentionally not modeled as async bridges:

- plain `await`
- promise chaining without parallel work
- other in-function suspension that does not hand work to a separate execution unit

Ambiguous async-looking constructs remain diagnostics instead of being converted into fake edges.

## Output Layout

The exporter writes:

```text
artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/review/
  00_index.md
  resolved_targets.json
  run_manifest.json
  flows/
    01_<slug>.md
    02_<slug>.md
    ...
  threads/
    00_index.md
    01_<slug>/
      00_overview.md
      focus/
        01_file_<slug>.md
        02_class_<slug>.md
        ...
  summaries/
    98_orphans_and_residuals.md
    99_diagnostics.md
  test-report/
    summary.md
    coverage.out
    coverage.html
    ...
```

The SQLite database lives here:

```text
artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/sqlite/review_graph.sqlite
```

## How To Read The Review

### `00_index.md`

Start here for:

- snapshot-level graph counts
- number of generated flows
- total graph coverage by exported flows
- residual and diagnostic totals

### `flows/*.md`

These are the main review artifacts.

Each flow file contains:

- grouped synchronous flow
- grouped asynchronous bridge sections
- affected files
- cross-service touchpoints
- ambiguities and import diagnostics
- coverage and cycle summary

### `threads/*.md`

These are additive companion artifacts for the same target slices.

- `threads/00_index.md` links each target to its merged overview and focus views
- `00_overview.md` shows the whole target slice merged at the file, module, and class bucket level
- `focus/*.md` centers the Mermaid view on one local file, module, or class/type bucket
- sync and async Mermaid graphs are rendered in separate sections so the merged thread stays readable

### `98_orphans_and_residuals.md`

Use this to see what was not covered by the selected targets and where the remaining graph is concentrated.

### `99_diagnostics.md`

Use this to inspect:

- weak async matches
- dropped ignored/test/generated content
- quality gaps imported from snapshot analysis

## Legacy Notes

- `scripts/run-review-graph.sh` and `scripts/run-review-graph-3layer.sh` have been removed.
- `graph plan-layering` and `graph apply-layering` are no longer part of the supported review flow.
- Older snapshots may still contain legacy layering artifacts or sidecar tables; the current grouped exporter ignores them.
