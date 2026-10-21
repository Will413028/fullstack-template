#!/usr/bin/env bash
# Fails when api/openapi.yaml has a breaking change against the base's copy (the contract's clients rely on it). The
# base is the merge-base with the given ref, so commits that landed on the base after this branch started are not
# reported here.
#
#   check-openapi-breaking.sh <oasdiff command> <base-ref>
#
# The oasdiff command is passed in so the version is pinned only in the Makefile (tools/go.mod).
set -euo pipefail
cd "$(dirname "$0")/.."
# A git hook exports GIT_DIR; without a work tree git would then take this directory as the repository root. Find the
# repository from here instead, and name paths from its root.
unset GIT_DIR GIT_WORK_TREE
prefix=$(git rev-parse --show-prefix)

oasdiff=${1:?usage: check-openapi-breaking.sh <oasdiff command> <base-ref>}
base=${2:?usage: check-openapi-breaking.sh <oasdiff command> <base-ref>}
merge_base=$(git merge-base HEAD "$base")
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
# Only a path missing from the base tree means "nothing to compare"; any git error fails the check (set -e).
listed=$(git ls-tree --full-tree --name-only "$merge_base" -- "${prefix}api/openapi.yaml")
if [[ -z "$listed" ]]; then
  echo "the base has no api/openapi.yaml, skipping the breaking-change check"
  exit 0
fi
git show "$merge_base:${prefix}api/openapi.yaml" >"$tmp"
# shellcheck disable=SC2086 # the command is a word list such as "go tool -modfile=tools/go.mod oasdiff"
$oasdiff breaking --fail-on ERR "$tmp" api/openapi.yaml
echo "no breaking change against $base"
