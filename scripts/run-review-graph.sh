#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_WORKSPACE="$(cd "$ROOT_DIR/.." && pwd)"
WORKSPACE_PATH="${1:-$DEFAULT_WORKSPACE}"
STARTPOINT_MODE="${STARTPOINT_MODE:-entrypoints}"

if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN="python3"
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN="python"
else
  echo "error: python3 or python is required to parse build-snapshot JSON output" >&2
  exit 1
fi

cd "$ROOT_DIR"

snapshot_json="$(mktemp)"
trap 'rm -f "$snapshot_json"' EXIT

echo "==> Building fresh snapshot for workspace: $WORKSPACE_PATH"
go run -mod=mod ./cmd/analysis-cli build-snapshot \
  --workspace "$WORKSPACE_PATH" \
  --progress-mode plain | tee "$snapshot_json"

read -r workspace_id snapshot_id <<EOF
$("$PYTHON_BIN" - "$snapshot_json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)

print(data["workspace_id"], data["snapshot"]["id"])
PY
)
EOF

db_path="artifacts/workspaces/$workspace_id/snapshots/$snapshot_id/sqlite/review_graph.sqlite"
review_dir="artifacts/workspaces/$workspace_id/snapshots/$snapshot_id/review"
targets_file="$review_dir/resolved_targets.json"

echo
echo "==> Importing review graph into: $db_path"
go run -mod=mod ./cmd/analysis-cli graph import-sqlite \
  --workspace-id "$workspace_id" \
  --snapshot-id "$snapshot_id"

echo
echo "==> Resolving startpoints with mode: $STARTPOINT_MODE"
go run -mod=mod ./cmd/analysis-cli graph list-startpoints \
  --db "$db_path" \
  --mode "$STARTPOINT_MODE"

echo
echo "==> Exporting markdown review into: $review_dir"
go run -mod=mod ./cmd/analysis-cli graph export-markdown-review \
  --db "$db_path" \
  --targets-file "$targets_file" \
  --mode full-flow \
  --include-async

echo
echo "Review graph run completed."
echo "workspace_id: $workspace_id"
echo "snapshot_id : $snapshot_id"
echo "db          : $db_path"
echo "targets     : $targets_file"
echo "review      : $review_dir"
