#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_WORKSPACE="$(cd "$ROOT_DIR/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
TEST_REPORT_OUT="${TEST_REPORT_OUT:-}"

usage() {
	cat <<'EOF'
Usage:
  bash ./scripts/run-review-report.sh --workspace <path> --symbol <name> [options]

New flow: index -> inspect-function -> review-flow -> export-* --review

Options:
  --workspace <path>    Workspace path (required)
  --symbol <name>       Symbol canonical name or ID to review (required)
  --progress-mode <m>   Progress mode: auto|tty|plain|quiet (default: plain)
  --out-dir <dir>       Output directory for review artifacts (optional)

Environment variables:
  GO_BIN            Go binary path. Default: go
  TEST_REPORT_OUT   Optional output directory for scripts/test_report.sh

Example:
  bash ./scripts/run-review-report.sh --workspace /path/to/workspace --symbol "github.com/example/pkg.MyFunc"
EOF
}

ensure_go() {
	if ! "$GO_BIN" version >/dev/null 2>&1; then
		echo "error: go is required to run the review report flow (set GO_BIN if go is installed outside PATH)" >&2
		exit 1
	fi
}

json_field() {
	local json_file="$1"
	local dotted_path="$2"
	python3 - "$json_file" "$dotted_path" <<'PY'
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

# Parse arguments
WORKSPACE_PATH=""
SYMBOL=""
PROGRESS_MODE="plain"
OUT_DIR=""
TEMP_PARENT=""  # Track temp parent for cleanup (only set when --out-dir not provided)

while [[ $# -gt 0 ]]; do
	case "$1" in
		-h|--help)
			usage
			exit 0
			;;
		--workspace)
			WORKSPACE_PATH="$2"
			shift
			;;
		--symbol)
			SYMBOL="$2"
			shift
			;;
		--progress-mode)
			PROGRESS_MODE="$2"
			shift
			;;
		--out-dir)
			OUT_DIR="$2"
			shift
			;;
		*)
			echo "error: unknown argument $1" >&2
			usage
			exit 1
			;;
	esac
	shift
done

# Validate required arguments
if [[ -z "$WORKSPACE_PATH" ]]; then
	echo "error: --workspace is required" >&2
	usage
	exit 1
fi

if [[ -z "$SYMBOL" ]]; then
	echo "error: --symbol is required" >&2
	usage
	exit 1
fi

ensure_go

cd "$ROOT_DIR"

echo "==> Indexing workspace: $WORKSPACE_PATH"
index_json="$(mktemp)"

# Setup cleanup for temp artifacts (only when --out-dir not provided)
cleanup_temp() {
	if [[ -n "$TEMP_PARENT" && -d "$TEMP_PARENT" ]]; then
		rm -rf "$TEMP_PARENT" >/dev/null 2>&1 || true
	fi
}
trap cleanup_temp EXIT

"$GO_BIN" run -mod=readonly ./cmd/analysis-cli index \
	--workspace "$WORKSPACE_PATH" \
	--progress-mode "$PROGRESS_MODE" | tee "$index_json"

# Parse workspace_id and snapshot_id from index output
workspace_id="$(json_field "$index_json" "workspace_id" | tr -d '\r')"
snapshot_id="$(json_field "$index_json" "snapshot_id" | tr -d '\r')"

if [[ -z "$workspace_id" || -z "$snapshot_id" ]]; then
	echo "error: failed to extract workspace_id or snapshot_id from index output" >&2
	exit 1
fi

echo
echo "==> Running inspect-function for symbol: $SYMBOL"
"$GO_BIN" run -mod=readonly ./cmd/analysis-cli inspect-function \
	--workspace-id "$workspace_id" \
	--snapshot-id "$snapshot_id" \
	--symbol "$SYMBOL" \
	--context-window 8

echo
echo "==> Running review-flow for symbol: $SYMBOL"
# Respect --out-dir if provided, otherwise use temp directory with cleanup
if [[ -n "$OUT_DIR" ]]; then
	# Use --out-dir directly as the review directory
	review_dir="$OUT_DIR"
else
	# Create temp directory that will be cleaned up on exit
	TEMP_PARENT="$(mktemp -d)"
	review_dir="$TEMP_PARENT/review"
fi
mkdir -p "$review_dir"

"$GO_BIN" run -mod=readonly ./cmd/analysis-cli review-flow \
	--workspace-id "$workspace_id" \
	--snapshot-id "$snapshot_id" \
	--symbol "$SYMBOL" \
	--max-depth 3 \
	--max-steps 80 \
	--out "$review_dir"

flow_json="$review_dir/flow.json"

echo
echo "==> Exporting review artifacts from: $flow_json"
"$GO_BIN" run -mod=readonly ./cmd/analysis-cli export-md --review "$flow_json"
"$GO_BIN" run -mod=readonly ./cmd/analysis-cli export-mermaid --review "$flow_json"
"$GO_BIN" run -mod=readonly ./cmd/analysis-cli export-graphjson --review "$flow_json"

# Run test report if TEST_REPORT_OUT is set
if [[ -n "$TEST_REPORT_OUT" ]]; then
	echo
	echo "==> Running test report into: $TEST_REPORT_OUT"
	GO_BIN="$GO_BIN" bash "$ROOT_DIR/scripts/test_report.sh" "$TEST_REPORT_OUT"
fi

echo
echo "Review report completed."
echo "workspace_id : $workspace_id"
echo "snapshot_id  : $snapshot_id"
echo "symbol       : $SYMBOL"
echo "review_dir   : $review_dir"
if [[ -n "$TEST_REPORT_OUT" ]]; then
	echo "test_report  : $TEST_REPORT_OUT"
fi

exit 0
