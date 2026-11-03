#!/usr/bin/env bash
# Fails when the contract in api/ has a breaking change against the base's copy (api/buf.yaml lists the rules). The
# base is the merge-base with the given ref, so commits that landed on the base after this branch started are not
# reported here.
#
#   check-buf-breaking.sh <buf command> <base-ref>
#
# The buf command is passed in so the version is pinned only in the root Makefile (make bin/buf).
set -euo pipefail
buf=${1:?usage: check-buf-breaking.sh <buf command> <base-ref>}
base=${2:?usage: check-buf-breaking.sh <buf command> <base-ref>}
# A path to buf is relative to the caller's directory (backend/Makefile passes ../bin/buf): resolve it before the cd.
if [[ $buf == */* ]]; then buf=$(cd "$(dirname "$buf")" && pwd)/$(basename "$buf"); fi
cd "$(dirname "$0")/.."
# A git hook exports GIT_DIR; without a work tree git would then take this directory as the repository root. Find the
# repository from here instead.
unset GIT_DIR GIT_WORK_TREE
merge_base=$(git merge-base HEAD "$base")
# Only a path missing from the base tree means "nothing to compare"; any git error fails the check (set -e).
listed=$(git ls-tree --full-tree --name-only "$merge_base" -- api/buf.yaml)
if [[ -z "$listed" ]]; then
  echo "the base has no api/buf.yaml, skipping the breaking-change check"
  exit 0
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# The base's api/ as plain files: works in a linked worktree and needs no buf support for git inputs.
git archive "$merge_base" api | tar -x -C "$tmp"
"$buf" breaking api --against "$tmp/api"
echo "no breaking change against $base"
