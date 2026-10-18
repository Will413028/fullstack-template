#!/usr/bin/env bash
# Fails when api/openapi.yaml has a breaking change against the base's copy (plan invariant I1). The base is the
# merge-base with the given ref, so commits that landed on the base after this branch started are not reported here.
#
#   check-openapi-breaking.sh <oasdiff command> <base-ref>
#
# The oasdiff command is passed in so the version is pinned only in the Makefile.
set -euo pipefail
cd "$(dirname "$0")/.."

oasdiff=${1:?usage: check-openapi-breaking.sh <oasdiff command> <base-ref>}
base=${2:?usage: check-openapi-breaking.sh <oasdiff command> <base-ref>}
merge_base=$(git merge-base HEAD "$base")
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
if ! git show "$merge_base:./api/openapi.yaml" >"$tmp" 2>/dev/null; then
  echo "the base has no api/openapi.yaml, skipping the breaking-change check"
  exit 0
fi
# shellcheck disable=SC2086 # the command is a word list such as "go run …@v1.33.0"
$oasdiff breaking --fail-on ERR "$tmp" api/openapi.yaml
echo "no breaking change against $base"
