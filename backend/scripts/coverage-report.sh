#!/usr/bin/env bash
# Prints a Markdown coverage report from coverage.out (written by make test). Coverage is reported, not gated
# (AGENTS.md Verification): this script fails only when the report cannot be produced. Generated files are left out.
#
#   coverage-report.sh <gocover-cobertura command> <diff-cover command> [base-ref]
#
# With a base ref it adds diff-cover's changed-line coverage against that ref. The commands are passed in so the
# versions are pinned only in the Makefile.
set -euo pipefail
cd "$(dirname "$0")/.."

usage="usage: coverage-report.sh <gocover-cobertura command> <diff-cover command> [base-ref]"
cobertura=${1:?$usage}
diff_cover=${2:?$usage}
base=${3:-}
[[ -f coverage.out ]] || { echo "no coverage.out: run make test first" >&2; exit 1; }

# shellcheck disable=SC2086 # the commands are word lists such as "go run …@v1.5.0"
$cobertura -ignore-gen-files -by-files <coverage.out >coverage.xml
python3 -I - coverage.xml <<'EOF'
import sys
import xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
rate = float(root.get("line-rate")) * 100
print("## Backend coverage\n")
print(f"Total line coverage: **{rate:.1f}%** ({root.get('lines-covered')}/{root.get('lines-valid')} lines, generated code excluded)\n")
EOF
[[ -n "$base" ]] || exit 0
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
# shellcheck disable=SC2086
$diff_cover coverage.xml --compare-branch="$base" --format "markdown:$tmp" >/dev/null
cat "$tmp"
