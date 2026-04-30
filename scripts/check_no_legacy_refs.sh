#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

blocked=(
  "internal/legacy"
  "reviewgraph"
  "reviewflow"
  "mermaid_old"
  "build-snapshot"
  "build-review-bundle"
  "blast-radius"
  "impacted-tests"
  "build-all-mermaid"
  "graph import-sqlite"
  "graph list-startpoints"
  "graph export-markdown-review"
  "HTTPHandler"
  "graphstore"
  "internal/store/"
  "workflows/facts_index"
  "workflows/build_snapshot"
  "workflows/export_mermaid"
  "domain/packet"
  "analysis-module/internal/workflows"
  "analysis-module/internal/store"
  "domain/flow"
  "workspace_scan"
)

# Explicit blocked paths check
blocked_paths=(
  "./internal/domain/flow"
  "./internal/domain/packet"
  "./internal/store"
  "./internal/workflows"
  "./internal/adapters/api"
  "./internal/adapters/cache"
  "./internal/ports/api"
  "./internal/ports/cache"
  "./internal/ports/query"
)

for path in "${blocked_paths[@]}"; do
  if [ -e "$path" ]; then
    echo "legacy path exists: $path"
    exit 1
  fi
done

for term in "${blocked[@]}"; do
  if grep -R "$term" ./cmd ./internal ./README.md 2>/dev/null; then
    echo "legacy reference found: $term"
    exit 1
  fi
done


echo "no active legacy references found"