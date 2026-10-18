#!/usr/bin/env bash
# Lock and safety checks for migrations/*.sql with squawk (config in .squawk.toml). Atlas runs each file in a
# transaction unless its header is `-- atlas:txmode none`, which squawk does not read: those files are checked with
# --no-assume-in-transaction, or a file that mixes CONCURRENTLY with other changes fails
# ban-concurrent-index-creation-in-transaction although Atlas applies it outside a transaction.
#
#   squawk.sh <squawk command...>
set -euo pipefail
cd "$(dirname "$0")/.."

(($# > 0)) || { echo "usage: squawk.sh <squawk command...>" >&2; exit 2; }
shopt -s nullglob
tx=() notx=()
for f in migrations/*.sql; do
  if head -n 1 "$f" | grep -qx -- '-- atlas:txmode none'; then notx+=("$f"); else tx+=("$f"); fi
done
if ((${#tx[@]} + ${#notx[@]} == 0)); then
  echo "no migrations, skipping squawk"
  exit 0
fi
if ((${#tx[@]} > 0)); then "$@" --config .squawk.toml "${tx[@]}"; fi
if ((${#notx[@]} > 0)); then "$@" --config .squawk.toml --no-assume-in-transaction "${notx[@]}"; fi
