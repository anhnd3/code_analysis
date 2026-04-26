#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_WORKSPACE="$(cd "$ROOT_DIR/.." && pwd)"
WORKSPACE_PATH="${1:-$DEFAULT_WORKSPACE}"
STARTPOINT_MODE="${STARTPOINT_MODE:-workflow}"
RENDER_MODE="${RENDER_MODE:-grouped}"
COMPANION_VIEW="${COMPANION_VIEW:-none}"
GO_BIN="${GO_BIN:-go}"
PYTHON_BIN=""

usage() {
	cat <<'EOF'
Usage:
  bash ./scripts/run-review-report.sh [workspace]

Legacy compatibility only:
  This script drives the old review-graph export path.

Environment variables:
  STARTPOINT_MODE   Startpoint selection mode. Default: workflow
  RENDER_MODE       Review render mode. Default: grouped
  COMPANION_VIEW    Companion thread views. Default: none
  GO_BIN            Go binary path. Default: go
  TEST_REPORT_OUT   Optional output directory for scripts/test_report.sh

Example:
  bash ./scripts/run-review-report.sh /mnt/d/Workspace/Local_Agent/software_agent_src/analysis_module/
EOF
}

ensure_go() {
	if ! "$GO_BIN" version >/dev/null 2>&1; then
		echo "error: go is required to run the review report flow (set GO_BIN if go is installed outside PATH)" >&2
		exit 1
	fi
}

ensure_python() {
	if [[ -n "$PYTHON_BIN" ]]; then
		return 0
	fi
	if command -v python3 >/dev/null 2>&1; then
		PYTHON_BIN="python3"
	elif command -v python >/dev/null 2>&1; then
		PYTHON_BIN="python"
	else
		echo "error: python3 or python is required to parse CLI JSON output" >&2
		exit 1
	fi
}

json_field() {
	local json_file="$1"
	local dotted_path="$2"
	"$PYTHON_BIN" - "$json_file" "$dotted_path" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    payload = json.load(fh)

value = payload
for part in sys.argv[2].split("."):
    value = value[part]

if value is None:
    print("")
elif isinstance(value, bool):
    print("true" if value else "false")
else:
    print(value)
PY
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

ensure_go
ensure_python
cd "$ROOT_DIR"

snapshot_json="$(mktemp)"
trap 'rm -f "$snapshot_json"' EXIT

echo "==> Building fresh snapshot for legacy reviewgraph workspace: $WORKSPACE_PATH"
"$GO_BIN" run -mod=mod ./cmd/analysis-cli build-snapshot \
	--workspace "$WORKSPACE_PATH" \
	--progress-mode plain | tee "$snapshot_json"

workspace_id="$(json_field "$snapshot_json" "workspace_id")"
snapshot_id="$(json_field "$snapshot_json" "snapshot.id")"
db_path="artifacts/workspaces/$workspace_id/snapshots/$snapshot_id/sqlite/review_graph.sqlite"
review_dir="artifacts/workspaces/$workspace_id/snapshots/$snapshot_id/review"
targets_file="$review_dir/resolved_targets.json"

echo
echo "==> Importing legacy review graph into: $db_path"
"$GO_BIN" run -mod=mod ./cmd/analysis-cli graph import-sqlite \
	--workspace-id "$workspace_id" \
	--snapshot-id "$snapshot_id"

echo
echo "==> Resolving legacy startpoints with mode: $STARTPOINT_MODE"
"$GO_BIN" run -mod=mod ./cmd/analysis-cli graph list-startpoints \
	--db "$db_path" \
	--mode "$STARTPOINT_MODE"

echo
echo "==> Exporting legacy markdown review into: $review_dir"
"$GO_BIN" run -mod=mod ./cmd/analysis-cli graph export-markdown-review \
	--db "$db_path" \
	--targets-file "$targets_file" \
	--mode full-flow \
	--render-mode "$RENDER_MODE" \
	--companion-view "$COMPANION_VIEW" \
	--include-async

test_report_out="${TEST_REPORT_OUT:-$review_dir/test-report}"
echo
echo "==> Running test report into: $test_report_out"
GO_BIN="$GO_BIN" bash "$ROOT_DIR/scripts/test_report.sh" "$test_report_out"

echo
echo "Review report completed."
echo "workspace_id : $workspace_id"
echo "snapshot_id  : $snapshot_id"
echo "db           : $db_path"
echo "targets      : $targets_file"
echo "review       : $review_dir"
echo "test report  : $test_report_out"
