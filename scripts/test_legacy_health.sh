#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GO_BIN="${GO_BIN:-go}"

packages=(
	./internal/services/reviewgraph_import
	./internal/services/reviewgraph_export
	./internal/services/reviewgraph_select
	./internal/services/reviewgraph_traverse
	./internal/services/reviewgraph_paths
	./internal/services/reviewflow_build
	./internal/services/reviewflow_expand
	./internal/services/reviewflow_policy
	./internal/services/flow_stitch
	./internal/services/chain_reduce
	./internal/services/sequence_model_build
	./internal/services/mermaid_emit
	./internal/workflows/export_mermaid
	./internal/workflows/review_graph_import
	./internal/workflows/review_graph_export
	./internal/workflows/review_graph_list_startpoints
	./internal/tests
)

"$GO_BIN" test -mod=mod "${packages[@]}"
