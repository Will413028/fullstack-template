#!/usr/bin/env bash
# Fails when a function is reached by neither a main package nor any test. deadcode prints such functions and still
# exits 0, so any output is a failure here.
#
#   deadcode.sh <deadcode command...>
set -euo pipefail
out=$("$@" -test ./...)
if [[ -n $out ]]; then
  echo "$out"
  echo "unreachable functions: delete them or call them" >&2
  exit 1
fi
