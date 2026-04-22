#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ROOT="$(cd "$ROOT_DIR/../.." && pwd)"
AUDIT_ROOT="$WORKSPACE_ROOT/audit/consumer_experience/scan"
OUT_ROOT="${OUT_ROOT:-$ROOT_DIR/artifacts/execution_b_validation}"
GO_BIN="${GO_BIN:-go}"

usage() {
	cat <<'EOF'
Usage:
  bash ./scripts/run_execution_b_validation_closeout.sh

Environment variables:
  GO_BIN    Go binary path. Default: go
  OUT_ROOT  Stable debug output root. Default: ./artifacts/execution_b_validation

This script:
  - runs strict review exports root-by-root
  - uses temporary artifact roots / SQLite paths
  - writes debug bundles into a stable repo path
  - stores CLI JSON output beside each debug bundle
EOF
}

run_export() {
	local workspace_path="$1"
	local root_selector="$2"
	local out_dir="$3"
	local artifact_root
	local sqlite_path
	local status

	artifact_root="$(mktemp -d)"
	sqlite_path="$(mktemp -u /tmp/execution_b_validation_XXXXXX.sqlite)"

	mkdir -p "$out_dir"
	echo "==> export-mermaid :: $workspace_path :: $root_selector"

	set +e
	(
		cd "$ROOT_DIR"
		ANALYSIS_ARTIFACT_ROOT="$artifact_root" \
		ANALYSIS_SQLITE_PATH="$sqlite_path" \
		env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod \
			"$GO_BIN" run -mod=mod ./cmd/analysis-cli export-mermaid \
			--workspace "$workspace_path" \
			--root-type http \
			--root-selector "$root_selector" \
			--render-mode review \
			--review-strict \
			--include-candidates \
			--emit-debug-bundle \
			--debug-out "$out_dir" \
			>"$out_dir/export_result.json"
	)
	status=$?
	set -e

	rm -rf "$artifact_root"
	rm -f "$sqlite_path"

	if [[ $status -ne 0 ]]; then
		echo "$status" >"$out_dir/export_status.txt"
		echo "FAILED: $root_selector" >&2
		return $status
	fi

	echo "0" >"$out_dir/export_status.txt"
	return 0
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

mkdir -p "$OUT_ROOT"

run_export \
	"$AUDIT_ROOT/zpa-camera-config-be" \
	"POST /v1/camera/detect-qr" \
	"$OUT_ROOT/zpa_camera_config_be/post_v1_detect_qr"

run_export \
	"$AUDIT_ROOT/zpa-camera-config-be" \
	"POST /v2/camera/detect-qr" \
	"$OUT_ROOT/zpa_camera_config_be/post_v2_detect_qr"

run_export \
	"$AUDIT_ROOT/zpa-camera-config-be" \
	"GET /v1/camera/config/all" \
	"$OUT_ROOT/zpa_camera_config_be/get_config_all"

run_export \
	"$AUDIT_ROOT/scan-everything-public-api" \
	"POST /scan360/v1/predict" \
	"$OUT_ROOT/scan_everything_public_api/post_scan360_v1_predict" || true

cat <<EOF

Execution B validation closeout exports completed.

Debug bundles:
  $OUT_ROOT/zpa_camera_config_be/post_v1_detect_qr
  $OUT_ROOT/zpa_camera_config_be/post_v2_detect_qr
  $OUT_ROOT/zpa_camera_config_be/get_config_all
  $OUT_ROOT/scan_everything_public_api/post_scan360_v1_predict

Curated semantic baselines:
  $AUDIT_ROOT/zpa-camera-config-be/graph-dependency/post_v1_detect_qr.mmd
  $AUDIT_ROOT/zpa-camera-config-be/graph-dependency/post_v2_detect_qr.mmd
  $AUDIT_ROOT/zpa-camera-config-be/graph-dependency/get_config_all.mmd
  $AUDIT_ROOT/scan-everything-public-api/graph-dependency/post_scan360_v1_predict.mmd
EOF
