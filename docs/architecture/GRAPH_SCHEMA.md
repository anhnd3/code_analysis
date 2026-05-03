# Review Graph Schema

## Purpose

The review graph is optimized for production impact review.

It is built to answer questions like:

- if a function changes, which runtime slice is reachable
- which async boundaries connect that function to topics, queues, schedulers, workers, or in-process concurrency
- which files and services a reviewer should inspect next

Traversal truth stays function-first. Grouping by file, class, or module happens only at markdown render time.

## Storage

The review graph is stored in a snapshot-local SQLite database:

```text
artifacts/workspaces/<workspace_id>/snapshots/<snapshot_id>/sqlite/review_graph.sqlite
```

The importer builds this database from existing snapshot artifacts:

- `graph_nodes.jsonl`
- `graph_edges.jsonl`
- `repository_manifests.json`
- `service_manifests.json`
- `quality_report.json` when present

## Core Tables

### `nodes`

Fields:

- `id TEXT PRIMARY KEY`
- `snapshot_id TEXT NOT NULL`
- `repo TEXT NOT NULL`
- `service TEXT`
- `language TEXT NOT NULL`
- `kind TEXT NOT NULL`
- `symbol TEXT NOT NULL`
- `file_path TEXT NOT NULL`
- `start_line INTEGER`
- `end_line INTEGER`
- `signature TEXT`
- `visibility TEXT`
- `node_role TEXT`
- `metadata_json TEXT`

### `edges`

Fields:

- `id TEXT PRIMARY KEY`
- `snapshot_id TEXT NOT NULL`
- `src_id TEXT NOT NULL`
- `dst_id TEXT NOT NULL`
- `edge_type TEXT NOT NULL`
- `flow_kind TEXT NOT NULL`
- `confidence REAL`
- `evidence_file TEXT`
- `evidence_line INTEGER`
- `evidence_text TEXT`
- `transport TEXT`
- `topic_or_channel TEXT`
- `metadata_json TEXT`

### `artifacts`

Fields:

- `id TEXT PRIMARY KEY`
- `snapshot_id TEXT NOT NULL`
- `artifact_type TEXT NOT NULL`
- `target_node_id TEXT`
- `path TEXT NOT NULL`
- `metadata_json TEXT`

## Indexes

The SQLite adapter creates indexes on:

- `nodes(symbol)`
- `nodes(file_path)`
- `nodes(service)`
- `nodes(node_role)`
- `edges(src_id)`
- `edges(dst_id)`
- `edges(edge_type)`
- `edges(flow_kind)`
- `edges(topic_or_channel)`

## Node Kinds

Runtime-relevant node kinds:

- `function`
- `method`
- `http_endpoint`
- `grpc_method`
- `event_topic`
- `pubsub_channel`
- `queue`
- `scheduler_job`
- `async_task`
- `inproc_channel`

Context node kinds:

- `class`
- `type`
- `module`
- `file`
- `service`

### Async V2 bridge meanings

- `event_topic`: event or topic-style external bridge such as Kafka
- `pubsub_channel`: named channel or subject-style external bridge such as Redis pubsub, NATS subject, or generic pubsub
- `queue`: job or queue-style external bridge such as RabbitMQ, SQS, Celery, or BullMQ
- `scheduler_job`: cron or scheduler-triggered execution unit
- `async_task`: spawned concurrent execution unit inside the process
- `inproc_channel`: in-process handoff primitive such as a Go channel, Python queue, or worker message lane

## Node Roles

The primary stored role values are:

- `normal`
- `entrypoint`
- `boundary`
- `public_api`
- `async_producer`
- `async_consumer`
- `scheduler`
- `shared_infra`

Role inference is based on imported manifests, repo/service hints, public visibility, and async bridge creation.

Only one `node_role` is stored on the row. Additional inferred roles may still appear in `metadata_json`.

Priority order is:

1. `async_producer`
2. `async_consumer`
3. `scheduler`
4. `entrypoint`
5. `boundary`
6. `public_api`
7. `shared_infra`
8. `normal`

## Edge Types

Structural/context edges:

- `DEFINES`
- `IMPORTS`
- `BELONGS_TO_SERVICE`

Synchronous runtime edge:

- `CALLS`

External async bridge edges:

- `EMITS_EVENT`
- `CONSUMES_EVENT`
- `PUBLISHES_MESSAGE`
- `SUBSCRIBES_MESSAGE`
- `ENQUEUES_JOB`
- `DEQUEUES_JOB`
- `SCHEDULES_TASK`
- `TRIGGERS_HTTP`

Async V2 native concurrency edges:

- `SPAWNS_ASYNC`
- `RUNS_ASYNC`
- `SENDS_TO_CHANNEL`
- `RECEIVES_FROM_CHANNEL`

### Traversal semantics

- sync traversal follows only `CALLS`
- async traversal follows only async bridge edges
- structural/context edges are preserved for metadata and summaries, not for visible sync call trees

## Flow Kinds

- `sync`
- `async`

## Deterministic IDs

Examples:

- file node: `file:<repo>:<file_path>`
- service node: `service:<service_name>`
- symbol node: `<language>:<repo>:<file_path>:<symbol_kind>:<canonical_symbol>`
- event topic: `event_topic:<transport>:<topic>`
- pubsub channel: `pubsub_channel:<transport>:<channel>`
- queue: `queue:<transport>:<queue>`
- scheduler job: `scheduler_job:<scope>:<job>`
- async task: `async_task:<scope>:<target>`
- in-process channel: `inproc_channel:<scope>:<channel>`

Edge IDs use:

```text
<src_id>-><edge_type>-><dst_id>
```

When multiple evidence rows collide on the same logical edge, a stable suffix is added.

Artifact IDs use a stable suffix derived from artifact type, target, and path.

## Import Rules

The importer is path-aware and production-focused.

Built-in exclusions include:

- `*_test.go`
- `tests/`
- `test/`
- `__tests__/`
- `*.spec.ts`
- `*.test.ts`
- `*.spec.js`
- `*.test.js`
- `dist/`
- `vendor/`
- `node_modules/`
- `artifacts/`
- `coverage/`
- `testdata/`

Additional ignore rules come from:

- workspace `.text-review-ignore`
- optional `--ignore-file`

## Async V2 Extraction

Async version recorded in the import manifest:

- `async-v2`

### General rule

Create async bridge edges only when the code clearly creates:

- a distinct concurrent execution unit, or
- an explicit message or handoff boundary

Do not create async bridge edges for plain `await`, promise chaining, or other intra-function suspension that does not create parallel or handed-off work.

### Go

Covered:

- `go namedFunc(...)`
- `go receiver.Method(...)`
- named channel send/receive patterns

Not promoted to strong edges:

- anonymous goroutines without a statically resolvable target

Those remain diagnostics.

### Python

Covered:

- `asyncio.create_task(...)`
- `asyncio.ensure_future(...)`
- `loop.create_task(...)`
- `asyncio.gather(...)` when the scheduled calls are statically resolvable
- `threading.Thread(target=...)`
- executor `.submit(...)`
- `queue.Queue` and `asyncio.Queue` handoff when statically attachable

### JS/TS

Covered:

- `new Worker(...)`
- `worker_threads.Worker(...)`
- explicit `postMessage` / `onmessage` or message-event style handoff

Plain `async` / `await` alone does not become an async bridge.

### Broker and scheduler detection

The importer combines literal/config detection with file/content transport hints for:

- Kafka
- RabbitMQ / AMQP
- Redis pubsub or streams
- NATS
- SQS / SNS
- Celery
- BullMQ
- generic queue/job names
- cron / scheduler registrations

When an async-looking construct is found without a resolvable target, it is recorded as a diagnostic instead of a strong flow claim.

## Export Artifacts

Current grouped review runs write:

- `import_manifest`
- `resolved_targets`
- `review_flow`
- `review_index`
- `review_thread_index`
- `review_thread_overview`
- `review_thread_focus`
- `review_residuals`
- `review_diagnostics`
- `run_manifest`

The grouped exporter writes markdown using `--render-mode grouped` by default. `--render-mode raw` is available for debugging. Companion Mermaid views are controlled with `--companion-view none|overview|all`, with `all` as the default.

## Grouped Rendering

Grouped rendering is derived from traversal results, not persisted back into SQLite.

The default presentation hierarchy is:

```text
file/module -> class/type -> function
```

Shared prefixes are merged deterministically and markers such as `root`, `leaf`, `shared_path`, `cycle`, and `truncated` stay attached to the rendered tree.

## Thread Companion Views

Companion views are derived from the same target traversal slices as the base flow files.

They are written under:

```text
review/threads/
  00_index.md
  01_<target-slug>/
    00_overview.md
    focus/
      01_file_<slug>.md
      02_class_<slug>.md
      03_module_<slug>.md
```

Rules:

- overview files merge the whole startpoint slice at the file, module, and class/type bucket level
- focus files are generated only for local file, module, or class/type buckets touched by that slice
- sync and async Mermaid graphs are rendered in separate sections
- companion views are additive only; they do not replace `review/flows/*.md`

## Legacy Compatibility

Older snapshots or databases may still contain:

- legacy layering artifact rows
- legacy `derived_nodes`
- legacy `derived_edges`

The current grouped review flow does not read or depend on those legacy layering structures.
