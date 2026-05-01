#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Check for legacy references first (must pass)
./scripts/check_no_legacy_refs.sh

GO_BIN="${GO_BIN:-go}"

packages=(
	./cmd/analysis-cli
	./internal/app
	./internal/indexer/...
	./internal/facts/...
	./internal/llm
	./internal/review
	./internal/export
	./internal/flow
)

"$GO_BIN" test -mod=mod "${packages[@]}"