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
)

for term in "${blocked[@]}"; do
  if grep -R "$term" ./cmd ./internal ./scripts ./README.md 2>/dev/null; then
    echo "legacy reference found: $term"
    exit 1
  fi
done

echo "no active legacy references found"