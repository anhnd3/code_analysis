#!/usr/bin/env bash
# check_no_legacy_refs.sh - Verify no references to legacy package structure remain
#
# This script checks for:
# 1. Legacy import paths in Go source files
# 2. Legacy directory structures that should have been flattened
# 3. Stale documentation references
#
# TODO 3 fix: Scans only active docs (README.md, docs/architecture), excludes docs/evidence

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

EXIT_CODE=0

echo "=== Checking for Legacy Package References ==="
echo "Root directory: $ROOT_DIR"
echo

# Track issues found
declare -a ISSUES_FOUND=()

###############################################
# SECTION 1: Check Go source files for legacy imports
###############################################

check_legacy_imports() {
	echo "--- Checking for legacy import paths in Go files ---"

	local patterns=(
		'internal/adapters'
		'internal/domain'
		'internal/ports'
		'internal/services'
	)

	local found_any=false
	for pattern in "${patterns[@]}"; do
		echo "  Searching for: $pattern"
		while IFS= read -r line; do
			if [[ -n "$line" ]]; then
				echo "    FOUND: $line"
				found_any=true
				ISSUES_FOUND+=("Legacy import '$pattern' found")
			fi
		done < <(grep -rn "$pattern" --include="*.go" "$ROOT_DIR" 2>/dev/null || true)
	done

	if [[ "$found_any" == "false" ]]; then
		echo "  ✓ No legacy imports found"
	else
		EXIT_CODE=1
	fi
}

###############################################
# SECTION 2: Check for legacy nested directories
###############################################

check_legacy_directories() {
	echo
	echo "--- Checking for legacy nested directory structures ---"

	# Check internal/facts for query/ and sqlite/ subdirectories
	echo "Checking internal/facts/ for nested dirs..."
	local facts_nested=$(find "$ROOT_DIR/internal/facts" -type d \( -name "query" -o -name "sqlite" \) 2>/dev/null | head -20)
	if [[ -n "$facts_nested" ]]; then
		echo "  ⚠ FOUND nested dirs in internal/facts:"
		echo "$facts_nested" | sed 's/^/    /'
		ISSUES_FOUND+=("Nested directories still exist in internal/facts/")
	else
		echo "  ✓ No nested query/sqlite dirs in internal/facts/"
	fi

	# Check internal/indexer for detector/, extractor/, boundary/ subdirectories
	echo "Checking internal/indexer/ for nested dirs..."
	local indexer_nested=$(find "$ROOT_DIR/internal/indexer" -type d \( -name "detector" -o -name "extractor" -o -name "boundary" \) 2>/dev/null | head -20)
	if [[ -n "$indexer_nested" ]]; then
		echo "  ⚠ FOUND nested dirs in internal/indexer:"
		echo "$indexer_nested" | sed 's/^/    /'
		ISSUES_FOUND+=("Nested directories still exist in internal/indexer/")
	else
		echo "  ✓ No nested detector/extractor/boundary dirs in internal/indexer/"
	fi

	# Check for facts/query and facts/sqlite patterns (old structure)
	echo "Checking for old 'facts/query' or 'facts/sqlite' paths..."
	local old_facts_paths=$(find "$ROOT_DIR/internal" -type d \( -path "*/facts/query*" -o -path "*/facts/sqlite*" \) 2>/dev/null | grep -v ".git" || true)
	if [[ -n "$old_facts_paths" ]]; then
		echo "  ⚠ FOUND old facts paths:"
		echo "$old_facts_paths" | sed 's/^/    /'
		ISSUES_FOUND+=("Old internal/facts/query or internal/facts/sqlite paths exist")
	else
		echo "  ✓ No legacy facts subdirectory paths found"
	fi

	# Check for indexer/detector, indexer/extractor, indexer/boundary (old structure)
	echo "Checking for old 'indexer/detector', etc. paths..."
	local old_indexer_paths=$(find "$ROOT_DIR/internal" -type d \( -path "*/indexer/detector*" -o -path "*/indexer/extractor*" -o -path "*/indexer/boundary*" \) 2>/dev/null | grep -v ".git" || true)
	if [[ -n "$old_indexer_paths" ]]; then
		echo "  ⚠ FOUND old indexer paths:"
		echo "$old_indexer_paths" | sed 's/^/    /'
		ISSUES_FOUND+=("Old internal/indexer/detector/etc. paths exist")
	else
		echo "  ✓ No legacy indexer subdirectory paths found"
	fi
}

###############################################
# SECTION 3: Check for legacy root directories
###############################################

check_legacy_root_dirs() {
	echo
	echo "--- Checking for legacy root-level directories ---"

	local legacy_roots=("pkg" "scratch" "schema" "schemas")

	for dir in "${legacy_roots[@]}"; do
		if [[ -d "$ROOT_DIR/$dir" ]]; then
			echo "  ⚠ FOUND $dir/ at root level"
			ISSUES_FOUND+=("Legacy '$dir' directory exists at root")
		fi
	done

	local dirs_found=$(ls -d "$ROOT_DIR"/pkg "$ROOT_DIR"/scratch "$ROOT_DIR"/schema "$ROOT_DIR"/schemas 2>/dev/null || true)
	if [[ -z "$dirs_found" ]]; then
		echo "  ✓ No legacy root directories (pkg/, scratch/, schema/, schemas/) found"
	else
		echo "  ⚠ Legacy root directories exist:"
		echo "$dirs_found" | sed 's/^/    /'
		EXIT_CODE=1
	fi
}

###############################################
# SECTION 4: Check documentation for stale references
# TODO 3: Only scan active docs, exclude docs/evidence
###############################################

check_documentation() {
	echo
	echo "--- Checking documentation for legacy references ---"

	local doc_patterns=(
		'internal/adapters'
		'internal/domain'
		'internal/ports'
		'internal/services'
	)

	# Scan only active docs: README.md, docs/architecture (exclude docs/evidence per TODO 3)
	# docs/evidence intentionally records quality-gate commands and should be excluded
	local doc_paths=(README.md)
	if [[ -d "$ROOT_DIR/docs/architecture" ]]; then
		doc_paths+=("docs/architecture")
	fi

	for pattern in "${doc_patterns[@]}"; do
		while IFS= read -r line; do
			if [[ -n "$line" ]]; then
				echo "  ⚠ Doc reference: $line"
				ISSUES_FOUND+=("Documentation still references '$pattern'")
			fi
		done < <(grep -rn "$pattern" --include="*.md" "${doc_paths[@]}" 2>/dev/null || true)
	done

	echo "  ✓ Documentation check complete"
}

###############################################
# SECTION 5: Active-reference guard (TODO 5)
###############################################
 check_active_legacy_refs() {
 	echo
 	echo "--- Checking active code for stale flattened-package and CLI references ---"

 	local legacy_patterns=(
 		'internal/facts/query/'
 		'internal/facts/sqlite/'
 		'internal/indexer/detector/'
 		'internal/indexer/extractor/'
 		'internal/indexer/boundary/'
 	)

	local legacy_commands=(
		'build-snapshot'
		'graph import-sqlite'
		'graph list-startpoints'
		'graph export-markdown-review'
	)

	# Scan active paths only: cmd, internal, README.md, docs/architecture, scripts.
	# Exclude this script and ADRs via explicit allowlist filtering.
	local scan_paths=(cmd internal README.md docs/architecture scripts)

	for pattern in "${legacy_patterns[@]}"; do
		while IFS= read -r line; do
			if [[ -n "$line" ]]; then
				echo "  ⚠ Stale ref: $line"
				ISSUES_FOUND+=("Active code references '$pattern'")
			fi
		done < <(grep -rn -- "$pattern" "${scan_paths[@]}" 2>/dev/null | grep -v 'check_no_legacy_refs.sh' | grep -vF 'ai_led_flow_architecture_decision_record.md' || true)
	done

	for cmd in "${legacy_commands[@]}"; do
		while IFS= read -r line; do
			if [[ -n "$line" ]]; then
				echo "  ⚠ Stale CLI ref: $line"
				ISSUES_FOUND+=("Active code references '$cmd'")
			fi
		done < <(grep -rn -- "$cmd" "${scan_paths[@]}" 2>/dev/null | grep -v 'check_no_legacy_refs.sh' | grep -vF 'ai_led_flow_architecture_decision_record.md' || true)
	done

	echo "  ✓ Active-reference check complete"
}

###############################################
# SECTION 6: Check for 'pkg/' imports (legacy pattern)
###############################################

check_legacy_pkg_imports() {
	echo
	echo "--- Checking for legacy pkg/ import patterns ---"

	# Only match actual import statements at start of line (with optional whitespace)
	# Exclude test files which may have string literals referencing pkg.
	local results=$(grep -rnE '^\s*["\w]*pkg/' --include="*.go" "$ROOT_DIR/internal" 2>/dev/null | grep -v "_test.go" || true)
	if [[ -n "$results" ]]; then
		echo "  ⚠ Found potential pkg/ imports:"
		echo "$results" | sed 's/^/    /'
		ISSUES_FOUND+=("Legacy pkg/ import found")
		EXIT_CODE=1
	else
		echo "  ✓ No legacy pkg/ imports in internal/"
	fi
}

###############################################
# Main execution
###############################################

cd "$ROOT_DIR"

check_legacy_imports
check_legacy_directories
check_legacy_root_dirs
check_documentation
check_active_legacy_refs
check_legacy_pkg_imports

echo
echo "========================================="
# TODO 3: Set EXIT_CODE=1 whenever ISSUES_FOUND is non-empty
if [[ ${#ISSUES_FOUND[@]} -gt 0 ]]; then
	EXIT_CODE=1
	echo "⚠ ISSUES FOUND (${#ISSUES_FOUND[@]} issues):"
	for issue in "${ISSUES_FOUND[@]}"; do
		if ! grep -qFx "$issue" <(printf '%s\n' "${ISSUES_FOUND[@]}" | sort -u); then
			true
		fi
		echo "  • $issue"
	done
else
	echo "✅ No legacy references found - all checks passed!"
fi
echo "========================================="

exit $EXIT_CODE
