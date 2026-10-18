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
files=(internal/modules/*/schema.sql)
if [[ ${#files[@]} -eq 0 ]]; then
  echo "no module schema.sql, skipping the drift check"
  exit 0
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp -R migrations/. "$tmp/"
before=$(ls "$tmp")
"$atlas" migrate diff drift_check --env dev --dir "file://$tmp"
if [[ "$(ls "$tmp")" != "$before" ]]; then
  cat "$tmp"/*_drift_check.sql
  echo "schema.sql and migrations/ differ: run make migration name=<name>" >&2
  exit 1
fi
echo "schema.sql and migrations/ are in sync"
