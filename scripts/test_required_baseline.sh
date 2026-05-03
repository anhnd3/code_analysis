#!/usr/bin/env bash
# test_required_baseline.sh — Quality gate baseline for analysis-module.
# Run from the module root; fails fast on any issue.
# Hardened: validates CLI path, uses temp artifact root, verifies JSON schema

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_WORKSPACE=""
TEMP_OUTPUT=""
CLEANUP_DONE=0

cleanup() {
  if [[ "$CLEANUP_DONE" -eq 0 ]]; then
    CLEANUP_DONE=1
    if [[ -n "$TEMP_WORKSPACE" ]] && [[ -d "$TEMP_WORKSPACE" ]]; then
      rm -rf "$TEMP_WORKSPACE" >/dev/null 2>&1 || true
    fi
    if [[ -n "$TEMP_OUTPUT" ]] && [[ -d "$TEMP_OUTPUT" ]]; then
      rm -rf "$TEMP_OUTPUT" >/dev/null 2>&1 || true
    fi
  fi
}

trap 'cleanup' EXIT
trap 'exit 1' ERR INT TERM

main() {
  echo "==> test_required_baseline.sh starting at $(date -Iseconds)"

  cd "$ROOT_DIR"

  # Use temp directory for all artifacts
  TEMP_WORKSPACE=$(mktemp -d)
  TEMP_OUTPUT=$(mktemp -d)

  # Set artifact root to temp directory to avoid repo-local writes
  export ANALYSIS_ARTIFACT_ROOT="$TEMP_OUTPUT/artifacts"

  echo "[0] script hygiene checks (CRLF guard)"
  local script
  for script in \
    "$ROOT_DIR/scripts/run-review-report.sh" \
    "$ROOT_DIR/scripts/test_required_baseline.sh" \
    "$ROOT_DIR/scripts/check_no_legacy_refs.sh"; do
    if rg -n $'\r' "$script" >/dev/null; then
      echo "ERROR: CRLF detected in $script" >&2
      rg -n $'\r' "$script" >&2
      exit 1
    fi
  done

  echo "[1] gofmt check (no stray files in cmd/internal)"
  if gofmt -l cmd internal | grep -q .; then
    echo "ERROR: gofmt -l cmd internal reported issues:"
    gofmt -l cmd internal
    exit 1
  fi

  echo "[2] legacy reference checks"
  ./scripts/check_no_legacy_refs.sh || {
    echo "ERROR: check_no_legacy_refs.sh failed" >&2
    exit 1
  }

  echo "[3] compile-only tests (no execution)"
  go test -run '^$' -mod=readonly ./cmd/... ./internal/... || {
    echo "ERROR: compile-only tests failed" >&2
    exit 1
  }

  echo "[4] full tests"
  go test -mod=readonly ./cmd/... ./internal/... || {
    echo "ERROR: full tests failed" >&2
    exit 1
  }

  echo "[5] build CLI to temp output"
  GOBIN="$TEMP_OUTPUT/bin" go build -mod=readonly -o "$TEMP_OUTPUT/bin/analysis-cli" ./cmd/analysis-cli || {
    echo "ERROR: failed to build analysis-cli" >&2
    exit 1
  }

  CLI="$TEMP_OUTPUT/bin/analysis-cli"
  FIXTURES="$ROOT_DIR/internal/tests/fixtures/single_go_service"

  # Use temp workspace for scanning/indexing
  TEMP_FIXTURE_WORKSPACE="$TEMP_WORKSPACE/fixtures"
  cp -r "$FIXTURES" "$TEMP_FIXTURE_WORKSPACE/"

  echo "[6] smoke test scan, index with temp workspace fixture"
  SCAN_OUTPUT="$TEMP_OUTPUT/scan.json"
  "$CLI" scan --workspace "$TEMP_FIXTURE_WORKSPACE" > "$SCAN_OUTPUT" || {
    echo "ERROR: scan failed" >&2
    exit 1
  }

  # Index and capture output for validation
  INDEX_OUTPUT="$TEMP_OUTPUT/index.json"
  "$CLI" index --workspace "$TEMP_FIXTURE_WORKSPACE" > "$INDEX_OUTPUT" || {
    echo "ERROR: index failed" >&2
    exit 1
  }

  # Parse workspace_id and snapshot_id from index JSON using Python
  PARSED_IDS=$(python3 -c "
import json
import sys
try:
    with open('$INDEX_OUTPUT', 'r') as f:
        data = json.load(f)
    ws_id = data.get('workspace_id', '')
    snap_id = data.get('snapshot_id', '')
    if ws_id and snap_id:
        print(f'{ws_id}:{snap_id}')
    else:
        sys.exit(1)
except Exception as e:
    print(f'ERROR: Failed to parse index JSON: {e}', file=sys.stderr)
    sys.exit(1)
" 2>&1 || {
    echo "ERROR: Could not parse workspace_id/snapshot_id from index output" >&2
    cat "$INDEX_OUTPUT" >&2
    exit 1
  })

  WORKSPACE_ID=$(echo "$PARSED_IDS" | cut -d':' -f1)
  SNAPSHOT_ID=$(echo "$PARSED_IDS" | cut -d':' -f2)

  echo "[7] inspect-function with parsed IDs (required success)"
  INSPECT_OUTPUT="$TEMP_OUTPUT/inspect.json"
  if ! "$CLI" inspect-function \
    --workspace-id "$WORKSPACE_ID" \
    --snapshot-id "$SNAPSHOT_ID" \
    --symbol "service.Handle" > "$INSPECT_OUTPUT" 2>&1; then
    echo "ERROR: inspect-function failed (required to succeed)" >&2
    cat "$INSPECT_OUTPUT" >&2
    exit 1
  fi

  # Validate inspect-function JSON schema
  echo "[8] validate inspect-function JSON schema"
  python3 -c "
import json
import sys

try:
    with open('$INSPECT_OUTPUT', 'r') as f:
        data = json.load(f)

    errors = []

    # Check root_symbol exists and has correct canonical_name
    root_symbol = data.get('root_symbol', {})
    if not root_symbol:
        errors.append('Missing root_symbol in response')
    else:
        if root_symbol.get('canonical_name') != 'service.Handle':
            errors.append(f\"Expected canonical_name='service.Handle', got '{root_symbol.get('canonical_name')}'\")

    # Check root_file exists and has non-empty relative_path
    root_file = data.get('root_file', {})
    if not root_file:
        errors.append('Missing root_file in response')
    elif not root_file.get('relative_path'):
        errors.append('root_file.relative_path is empty or missing')

    if errors:
        for err in errors:
            print(f'ERROR: {err}', file=sys.stderr)
        sys.exit(1)
    else:
        print('JSON schema validation passed')
except json.JSONDecodeError as e:
    print(f'ERROR: Invalid JSON in inspect output: {e}', file=sys.stderr)
    sys.exit(1)
" || {
    echo "ERROR: JSON schema validation failed" >&2
    exit 1
  }

  # Verify all generated artifact paths stay under ANALYSIS_ARTIFACT_ROOT
  echo "[9] verify no repo-local artifact pollution"
  python3 -c "
import json
import os
import sys

artifact_root = os.path.realpath('$ANALYSIS_ARTIFACT_ROOT')
scan_path = '$SCAN_OUTPUT'
index_path = '$INDEX_OUTPUT'

def add_path(bucket, label, path_value):
    if isinstance(path_value, str) and path_value:
        bucket.append((label, path_value))

paths = []

with open(scan_path, 'r', encoding='utf-8') as fh:
    scan_payload = json.load(fh)
for ref in scan_payload.get('artifact_refs', []):
    add_path(paths, 'scan.artifact_refs', ref.get('path'))

with open(index_path, 'r', encoding='utf-8') as fh:
    index_payload = json.load(fh)
add_path(paths, 'index.sqlite_path', index_payload.get('sqlite_path'))
for key, value in index_payload.get('jsonl', {}).items():
    add_path(paths, f'index.jsonl.{key}', value)
for ref in index_payload.get('artifact_refs', []):
    add_path(paths, 'index.artifact_refs', ref.get('path'))

if not paths:
    print('ERROR: no artifact paths found to validate', file=sys.stderr)
    sys.exit(1)

errors = []
for label, path_value in paths:
    resolved = os.path.realpath(path_value)
    try:
        common = os.path.commonpath([artifact_root, resolved])
    except ValueError:
        common = ''
    if common != artifact_root:
        errors.append(f'{label}: {path_value}')

if errors:
    print('ERROR: artifact paths escaped ANALYSIS_ARTIFACT_ROOT', file=sys.stderr)
    for item in errors:
        print(f'  - {item}', file=sys.stderr)
    sys.exit(1)

print(f'Artifact path validation passed ({len(paths)} paths under {artifact_root})')
" || {
    echo "ERROR: artifact root validation failed" >&2
    exit 1
  }

  echo "[10] export smoke for markdown, mermaid, graphjson with handcrafted ReviewFlow"
  FLOW_JSON="$TEMP_OUTPUT/flow.json"
  cat > "$FLOW_JSON" <<'EOF'
{
  "id": "test-flow-001",
  "workspace_id": "test-workspace",
  "snapshot_id": "test-snapshot",
  "root_symbol_id": "symbol-handle",
  "root_canonical_name": "analysis-module/internal/service.Handle",
  "created_at": "2025-01-01T00:00:00Z",
  "steps": [
    {
      "id": "step-1",
      "from_symbol_id": "symbol-handle",
      "from_canonical_name": "analysis-module/internal/service.Handle",
      "to_symbol_id": "symbol-stepone",
      "to_canonical_name": "analysis-module/internal/service.stepOne",
      "status": "accepted",
      "rationale": "direct call"
    }
  ],
  "accepted": [
    {
      "id": "step-1",
      "from_symbol_id": "symbol-handle",
      "from_canonical_name": "analysis-module/internal/service.Handle",
      "to_symbol_id": "symbol-stepone",
      "to_canonical_name": "analysis-module/internal/service.stepOne",
      "status": "accepted",
      "rationale": "direct call"
    }
  ],
  "ambiguous": [],
  "rejected": [],
  "uncertainty_notes": []
}
EOF

  "$CLI" export-md -review="$FLOW_JSON" >/dev/null || {
    echo "ERROR: export-md failed" >&2
    exit 1
  }
  "$CLI" export-mermaid -review="$FLOW_JSON" >/dev/null || {
    echo "ERROR: export-mermaid failed" >&2
    exit 1
  }
  "$CLI" export-graphjson -review="$FLOW_JSON" >/dev/null || {
    echo "ERROR: export-graphjson failed" >&2
    exit 1
  }

  echo "==> test_required_baseline.sh completed successfully at $(date -Iseconds)"
}

main "$@"
