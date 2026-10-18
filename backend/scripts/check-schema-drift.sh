#!/usr/bin/env bash
# The modules' schema.sql files and the state replayed from migrations/ must match: fails when a schema.sql was
# changed without generating a migration. Runs `atlas migrate diff --env dev` (the standard build, so atlas.hcl's
# source order applies: auth first) against a copy of migrations/, so the check never writes into the repository;
# any file it generates is drift. A tampered atlas.sum also fails here. Needs Docker (Atlas's dev database).
#
#   check-schema-drift.sh <atlas binary>
set -euo pipefail
cd "$(dirname "$0")/.."

atlas=${1:?usage: check-schema-drift.sh <atlas binary>}
shopt -s nullglob
schemas=(internal/modules/*/schema.sql)
migrations=(migrations/*.sql)
if ((${#schemas[@]} + ${#migrations[@]} == 0)); then
  echo "no module schema.sql and no migrations, skipping the drift check"
  exit 0
fi
if ((${#schemas[@]} == 0)); then
  echo "migrations/ has migrations but no module has a schema.sql" >&2
  exit 1
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp -R migrations/. "$tmp/"
before=$(ls "$tmp")
"$atlas" migrate diff drift_check --env dev --dir "file://$tmp"
added=$(comm -13 <(echo "$before") <(ls "$tmp") | grep -vx atlas.sum || true)
if [[ -n "$added" ]]; then
  for f in $added; do cat "$tmp/$f"; done
  echo "schema.sql and migrations/ differ: run make migration name=<name>" >&2
  exit 1
fi
echo "schema.sql and migrations/ are in sync"
