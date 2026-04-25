#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ROOT="$(cd "$ROOT_DIR/../.." && pwd)"
AUDIT_ROOT="$WORKSPACE_ROOT/audit/consumer_experience/scan"
OUT_ROOT="${OUT_ROOT:-$ROOT_DIR/artifacts/execution_c_service_pack}"
GO_BIN="${GO_BIN:-go}"

usage() {
	cat <<'EOF'
Usage:
  bash ./scripts/run_execution_c_service_pack.sh

Environment variables:
  GO_BIN    Go binary path. Default: go
  OUT_ROOT  Output debug root. Default: ./artifacts/execution_c_service_pack

This script:
  - runs export-mermaid in service_pack mode
  - uses isolated /tmp sqlite and artifact roots
  - uses isolated /tmp go cache and mod cache
  - emits debug bundles and JSON result files per service
  - copies snapshot artifacts (coverage, selected flows, per-root files) into each service output directory
EOF
}

copy_snapshot_artifacts() {
	local artifact_root="$1"
	local out_dir="$2"
	local snapshot_dir

	snapshot_dir="$(find "$artifact_root/workspaces" -mindepth 3 -maxdepth 3 -type d 2>/dev/null | head -n 1 || true)"
	if [[ -z "${snapshot_dir}" ]]; then
		echo "FAILED: unable to locate snapshot artifact directory under $artifact_root" >&2
		return 1
	fi

	cp -R "$snapshot_dir"/. "$out_dir"/
	return 0
}

run_service_pack() {
	local service_name="$1"
	local workspace_path="$2"
	local expected_roots_file="$3"
	local out_dir="$4"
	local artifact_root
	local sqlite_path
	local status

	artifact_root="$(mktemp -d /tmp/execution_c_service_pack_artifacts_XXXXXX)"
	sqlite_path="$(mktemp -u /tmp/execution_c_service_pack_XXXXXX.sqlite)"
	rm -rf "$out_dir"
	mkdir -p "$out_dir"

	echo "==> export-mermaid service_pack :: $service_name"
	set +e
	(
		cd "$ROOT_DIR"
		ANALYSIS_ARTIFACT_ROOT="$artifact_root" \
		ANALYSIS_SQLITE_PATH="$sqlite_path" \
		GOCACHE=/tmp/go-build \
		GOMODCACHE=/tmp/go-mod \
		"$GO_BIN" run -mod=mod ./cmd/analysis-cli export-mermaid \
			--workspace "$workspace_path" \
			--root-type master \
			--review-scope service_pack \
			--expected-roots-file "$expected_roots_file" \
			--render-mode review \
			--review-strict \
			--emit-debug-bundle \
			--debug-out "$out_dir" \
			--service-name "$service_name" \
			>"$out_dir/export_result.json"
	)
	status=$?
	set -e

	if [[ $status -eq 0 ]]; then
		if ! copy_snapshot_artifacts "$artifact_root" "$out_dir"; then
			status=1
		fi
	fi

	rm -rf "$artifact_root"
	rm -f "$sqlite_path"

	if [[ $status -ne 0 ]]; then
		echo "$status" >"$out_dir/export_status.txt"
		echo "FAILED: $service_name" >&2
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

run_service_pack \
	"zpa-camera-config-be" \
	"$AUDIT_ROOT/zpa-camera-config-be" \
	"$AUDIT_ROOT/zpa-camera-config-be/expected_roots.json" \
	"$OUT_ROOT/zpa_camera_config_be"

run_service_pack \
	"scan-everything-public-api" \
	"$AUDIT_ROOT/scan-everything-public-api" \
	"$AUDIT_ROOT/scan-everything-public-api/expected_roots.json" \
	"$OUT_ROOT/scan_everything_public_api"

cat <<EOF

Execution C service-pack run completed.

Debug bundles:
  $OUT_ROOT/zpa_camera_config_be
  $OUT_ROOT/scan_everything_public_api
EOF
