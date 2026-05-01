#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

blocked=(
  "internal/query"
  "internal/facts/store"
  "analysis-module/internal/query"
  "analysis-module/internal/facts/store"
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
  # Blocked terms for flattened flow subpackages
  "internal/flow/model"
  "internal/flow/trace"
  "internal/flow/evidence"
  "internal/flow/resolve"
  "internal/flow/export"
  "analysis-module/internal/flow/model"
  "analysis-module/internal/flow/trace"
  "analysis-module/internal/flow/evidence"
  "analysis-module/internal/flow/resolve"
  "analysis-module/internal/flow/export"
  # Blocked terms for flattened export subpackages
  "internal/export/markdown"
  "internal/export/mermaid"
  "analysis-module/internal/export/markdown"
  "analysis-module/internal/export/mermaid"
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
  # Blocked paths for flattened flow subpackages
  "./internal/flow/model"
  "./internal/flow/trace"
  "./internal/flow/evidence"
  "./internal/flow/resolve"
  "./internal/flow/export"
  # Blocked paths for flattened export subpackages
  "./internal/export/markdown"
  "./internal/export/mermaid"
)

for path in "${blocked_paths[@]}"; do
  if [ -e "$path" ]; then
    echo "legacy path exists: $path"
    exit 1
  fi
done

# Scan active code and README only. Docs are historical records that may legitimately
# reference retired concepts (e.g., the ADR documents why reviewflow was retired).
for term in "${blocked[@]}"; do
  if grep -R "$term" ./cmd ./internal ./README.md 2>/dev/null; then
    echo "legacy reference found: $term"
    exit 1
  fi
done

echo "no active legacy references found"
