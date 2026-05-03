# Review Flow Usage (Current)

> This document describes the supported review flow using the current CLI.

## Purpose

The review flow helps you understand:

- What runtime paths are affected if a function changes
- Which callers and callees matter in the reachable slice
- How async bridges connect code to queues, topics, schedulers, workers, or concurrency primitives
- Which files and services a reviewer should inspect next

## Supported Command Sequence

```text
scan -> index -> inspect-function -> review-flow -> export-md / export-mermaid / export-graphjson
```

### 1. Scan Workspace

Discover repositories and services in the workspace:

```bash
analysis-cli scan --workspace /path/to/workspace
```

Output includes `workspace_id` for subsequent commands.

### 2. Index Facts

Extract symbols, calls, and structural facts:

```bash
analysis-cli index --workspace /path/to/workspace
```

Parse `workspace_id` and `snapshot_id` from the output.

### 3. Inspect Function

Validate that a symbol exists and view its context:

```bash
analysis-cli inspect-function \
  --workspace-id <workspace_id> \
  --snapshot-id <snapshot_id> \
  --symbol 'package/path.Type.Method'
```

### 4. Run Review Flow

Generate the review flow JSON:

```bash
analysis-cli review-flow \
  --workspace-id <workspace_id> \
  --snapshot-id <snapshot_id> \
  --symbol 'package/path.Type.Method' \
  --max-depth 3 \
  --max-steps 80 \
  --out ./review-output
```

This creates:

| File | Description |
|------|-------------|
| `flow.json` | Accepted review flow with steps |
| `evidence.json` | Evidence for each step |
| `uncertainty.md` | Uncertainty notes from LLM analysis |

### 5. Export Artifacts

Generate human-readable and machine-readable exports:

```bash
# Markdown review
analysis-cli export-md --review ./review-output/flow.json --out ./review-output/flow.md

# Mermaid diagram
analysis-cli export-mermaid --review ./review-output/flow.json --out ./review-output/flow.mmd

# Graph JSON (Cytoscape/Graphviz compatible)
analysis-cli export-graphjson --review ./review-output/flow.json --out ./review-output/graph.json
```

## Output Structure

```text
./review-output/
  flow.json           — Accepted review flow
  evidence.json       — Step-by-step evidence
  uncertainty.md      — LLM uncertainty notes
  flow.md             — Markdown export
  flow.mmd            — Mermaid diagram
  graph.json          — Graph JSON for visualization
```

## Package Organization

The review flow uses these packages:

| Package | Role |
|---------|------|
| `internal/app` | Bootstrap, config, lifecycle |
| `internal/indexer` | Workspace scanning and symbol extraction |
| `internal/facts` | Fact model types and builders |
| `internal/flow` | Flow skeleton (types, trace, resolver, verifier) — Phase 4 root |
| `internal/review` | LLM-led review service (transitional; superseded by `internal/flow`) |
| `internal/export` | Export services for markdown, mermaid, graphjson |
| `internal/llm` | LLM integration |

## Transitional Note

The `internal/review` package remains transitional. Future phases will migrate its functionality into `internal/flow`.
