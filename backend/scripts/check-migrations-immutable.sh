#!/usr/bin/env bash
# Migrations that exist on the base must not be modified, deleted or renamed (docs/adr/2026-10-18-go-backend.md D2).
# The existing migrations are the files listed in the base's migrations/atlas.sum, compared file by file. atlas.sum
# itself is not compared because adding a migration always changes it; editing a file and then rehashing is still
# caught here.
#
#   check-migrations-immutable.sh <base-ref>
#
# Compares the working tree with the merge-base, so uncommitted changes are covered too.
set -euo pipefail
cd "$(dirname "$0")/.."
# A git hook exports GIT_DIR; without a work tree git would then take this directory as the repository root. Find the
# repository from here instead, and name paths from its root.
unset GIT_DIR GIT_WORK_TREE
prefix=$(git rev-parse --show-prefix)

base=${1:?usage: check-migrations-immutable.sh <base-ref>}
merge_base=$(git merge-base HEAD "$base")
# Only a path missing from the base tree means "nothing to compare"; any git error fails the check (set -e).
listed=$(git ls-tree --full-tree --name-only "$merge_base" -- "${prefix}migrations/atlas.sum")
if [[ -z "$listed" ]]; then
  echo "the base has no migrations/atlas.sum, so no existing migrations"
  exit 0
fi
sum=$(git show "$merge_base:${prefix}migrations/atlas.sum")
changed=()
while read -r name _; do
  [[ -z "$name" || "$name" == h1:* ]] && continue # the first line is the hash of the whole directory
  if [[ ! -f "migrations/$name" ]] || ! git show "$merge_base:${prefix}migrations/$name" | cmp -s - "migrations/$name"; then
    changed+=("$name")
  fi
done <<<"$sum"
if ((${#changed[@]} == 0)); then
  echo "no existing migration was modified"
  exit 0
fi
printf '%s\n' "${changed[@]}"
echo "existing migrations must not be modified, deleted or renamed; add a new migration" >&2
exit 1
