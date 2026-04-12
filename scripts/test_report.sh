#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

timestamp="$(date -u +%Y%m%d_%H%M%S)"
out_dir="${1:-$ROOT_DIR/artifacts/test_reports/$timestamp}"

mkdir -p "$out_dir"

json_file="$out_dir/go-test.jsonl"
coverage_file="$out_dir/coverage.out"
coverage_html_file="$out_dir/coverage.html"
test_cases_file="$out_dir/test-cases.tsv"
module_coverage_file="$out_dir/module-coverage.tsv"
file_coverage_file="$out_dir/file-coverage.tsv"
summary_file="$out_dir/summary.md"

printf 'module\tfiles\tcovered_statements\ttotal_statements\tcoverage_percent\n' > "$module_coverage_file"
printf 'module\tfile\tcovered_statements\ttotal_statements\tcoverage_percent\n' > "$file_coverage_file"

set +e
go test -mod=mod -json -covermode=atomic -coverpkg=./... -coverprofile="$coverage_file" ./... > "$json_file"
test_exit=$?
set -e

awk '
function capture_string(field, line,    pattern, m) {
	pattern = "\"" field "\":\"([^\"]+)\""
	if (match(line, pattern, m)) {
		return m[1]
	}
	return ""
}
function capture_number(field, line,    pattern, m) {
	pattern = "\"" field "\":([0-9.]+)"
	if (match(line, pattern, m)) {
		return m[1]
	}
	return ""
}
BEGIN {
	print "status\tpackage\ttest\telapsed_seconds"
}
{
	action = capture_string("Action", $0)
	test = capture_string("Test", $0)
	if (test == "") {
		next
	}
	if (action != "pass" && action != "fail" && action != "skip") {
		next
	}
	pkg = capture_string("Package", $0)
	elapsed = capture_number("Elapsed", $0)
	if (elapsed == "") {
		elapsed = "-"
	}
	printf "%s\t%s\t%s\t%s\n", toupper(action), pkg, test, elapsed
}
' "$json_file" > "$test_cases_file"

total_coverage="n/a"
covered_statements="0"
total_statements="0"
module_count="0"
file_count="0"
if [[ -f "$coverage_file" ]]; then
	go tool cover -html="$coverage_file" -o "$coverage_html_file"
	module_raw="$out_dir/module-coverage.raw.tsv"
	file_raw="$out_dir/file-coverage.raw.tsv"

	awk -v module_out="$module_raw" -v file_out="$file_raw" '
	BEGIN {
		FS = "[[:space:]]+"
		OFS = "\t"
		print "module\tfiles\tcovered_statements\ttotal_statements\tcoverage_percent" > module_out
		print "module\tfile\tcovered_statements\ttotal_statements\tcoverage_percent" > file_out
	}
	NR == 1 {
		next
	}
	{
		split($1, loc, ":")
		file = loc[1]
		sub(/^analysis-module\//, "", file)
		module = file
		sub(/\/[^/]+$/, "", module)
		if (module == file) {
			module = "."
		}
		stmts = $2 + 0
		count = $3 + 0
		total[file] += stmts
		if (count > 0) {
			covered[file] += stmts
		}
		module_total[module] += stmts
		if (count > 0) {
			module_covered[module] += stmts
		}
		if (!(seen[module SUBSEP file]++)) {
			module_files[module]++
		}
		all_total += stmts
		if (count > 0) {
			all_covered += stmts
		}
	}
	END {
		for (module in module_total) {
			pct = 0
			if (module_total[module] > 0) {
				pct = 100 * module_covered[module] / module_total[module]
			}
			printf "%s\t%d\t%d\t%d\t%.1f%%\n", module, module_files[module], module_covered[module], module_total[module], pct >> module_out
		}
		for (file in total) {
			module = file
			sub(/\/[^/]+$/, "", module)
			if (module == file) {
				module = "."
			}
			name = file
			sub(/^.*\//, "", name)
			pct = 0
			if (total[file] > 0) {
				pct = 100 * covered[file] / total[file]
			}
			printf "%s\t%s\t%d\t%d\t%.1f%%\n", module, name, covered[file], total[file], pct >> file_out
		}
		printf "%d\t%d\n", all_covered, all_total
	}
	' "$coverage_file" > "$out_dir/overall-coverage.raw"

	{
		head -n 1 "$module_raw"
		tail -n +2 "$module_raw" | sort -t $'\t' -k1,1
	} > "$module_coverage_file"
	{
		head -n 1 "$file_raw"
		tail -n +2 "$file_raw" | sort -t $'\t' -k1,1 -k2,2
	} > "$file_coverage_file"

	read -r covered_statements total_statements < "$out_dir/overall-coverage.raw"
	module_count="$(awk 'NR > 1 {count++} END {print count + 0}' FS=$'\t' "$module_coverage_file")"
	file_count="$(awk 'NR > 1 {count++} END {print count + 0}' FS=$'\t' "$file_coverage_file")"
	if [[ "$total_statements" -gt 0 ]]; then
		total_coverage="$(awk -v covered="$covered_statements" -v total="$total_statements" 'BEGIN { printf "%.1f%%", (100 * covered / total) }')"
	fi
fi

pass_count="$(awk 'NR > 1 && $1 == "PASS" {count++} END {print count + 0}' FS=$'\t' "$test_cases_file")"
fail_count="$(awk 'NR > 1 && $1 == "FAIL" {count++} END {print count + 0}' FS=$'\t' "$test_cases_file")"
skip_count="$(awk 'NR > 1 && $1 == "SKIP" {count++} END {print count + 0}' FS=$'\t' "$test_cases_file")"

{
	echo "# Test Report"
	echo
	echo "## Overall Coverage"
	echo
	echo "| Status | Passed | Failed | Skipped | Modules | Files | Covered Statements | Total Statements | Coverage |"
	echo "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |"
	printf "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n" \
		"$([[ $test_exit -eq 0 ]] && echo PASS || echo FAIL)" \
		"$pass_count" \
		"$fail_count" \
		"$skip_count" \
		"$module_count" \
		"$file_count" \
		"$covered_statements" \
		"$total_statements" \
		"$total_coverage"
	echo
	echo "## Module Coverage"
	echo
	echo "| Module | Files | Covered Statements | Total Statements | Coverage |"
	echo "| --- | ---: | ---: | ---: | ---: |"
	awk 'NR > 1 {printf "| `%s` | %s | %s | %s | %s |\n", $1, $2, $3, $4, $5}' FS=$'\t' "$module_coverage_file"
	echo
	echo "## File Coverage"
	echo
	echo "| Module | File | Covered Statements | Total Statements | Coverage |"
	echo "| --- | --- | ---: | ---: | ---: |"
	awk 'NR > 1 {printf "| `%s` | `%s` | %s | %s | %s |\n", $1, $2, $3, $4, $5}' FS=$'\t' "$file_coverage_file"
} > "$summary_file"

cat "$summary_file"

exit "$test_exit"
