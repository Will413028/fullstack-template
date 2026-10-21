#!/usr/bin/env bash
# Shows that the checks make check relies on still fail when they should. Each case builds what it needs in a temporary
# directory, so nothing here is tracked (a vulnerable fixture module in the repository would fail osv-scanner).
#
#   selftest-checks.sh
#
# oasdiff and govulncheck are built from tools/go.mod, the versions make check runs. A case that must fail passes only
# when the output names the expected reason, so a tool that did not start cannot count as a detection.
#
# - check-migrations-immutable.sh: a modified existing migration fails, also with GIT_DIR exported as in a git hook;
#   a base without migrations passes; an unknown base ref fails instead of passing as "nothing to compare".
# - check-openapi-breaking.sh: removing a response property fails.
# - govulncheck: a module that calls a known-vulnerable function fails.
set -euo pipefail
cd "$(dirname "$0")/.."
unset GIT_DIR GIT_WORK_TREE

scripts=$PWD/scripts
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failures=0
go build -modfile=tools/go.mod -o "$tmp/bin/" github.com/oasdiff/oasdiff golang.org/x/vuln/cmd/govulncheck
oasdiff=$tmp/bin/oasdiff
govulncheck=$tmp/bin/govulncheck

expect() { # expect pass <description> <command...> | expect fail <description> <output pattern> <command...>
  local want=$1 what=$2 pattern=""
  shift 2
  if [[ $want == fail ]]; then pattern=$1; shift; fi
  if "$@" >"$tmp/out" 2>&1; then got=pass; else got=fail; fi
  if [[ $got == "$want" && ( -z $pattern || $(grep -cE "$pattern" "$tmp/out") -gt 0 ) ]]; then
    echo "ok   $what"
  else
    echo "FAIL $what: expected $want${pattern:+ matching /$pattern/}, got $got"
    sed 's/^/     /' "$tmp/out"
    failures=$((failures + 1))
  fi
}

# A repository shaped like this one: the scripts and data live under backend/.
repo=$tmp/repo
mkdir -p "$repo/backend/scripts" "$repo/backend/migrations" "$repo/backend/api"
cp "$scripts/check-migrations-immutable.sh" "$scripts/check-openapi-breaking.sh" "$repo/backend/scripts/"
git -C "$repo" init -q -b main
git -C "$repo" -c user.name=t -c user.email=t@t commit -q --allow-empty -m empty
in_backend() { (cd "$repo/backend" && "$@"); }
with_git_dir() { (cd "$repo/backend" && GIT_DIR="$repo/.git" "$@"); }

expect pass "migrations: a base without migrations passes" in_backend scripts/check-migrations-immutable.sh HEAD

printf 'CREATE TABLE t (id int);\n' >"$repo/backend/migrations/1_init.sql"
printf 'h1:dir\n1_init.sql h1:file\n' >"$repo/backend/migrations/atlas.sum"
cat >"$repo/backend/api/openapi.yaml" <<'EOF'
openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /t:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                required: [id, name]
                properties:
                  id: {type: string}
                  name: {type: string}
EOF
git -C "$repo" add -A
git -C "$repo" -c user.name=t -c user.email=t@t commit -q -m base

expect pass "migrations: unchanged migrations pass" in_backend scripts/check-migrations-immutable.sh HEAD
expect pass "migrations: unchanged migrations pass with GIT_DIR" with_git_dir scripts/check-migrations-immutable.sh HEAD
printf -- '-- edited\n' >>"$repo/backend/migrations/1_init.sql"
expect fail "migrations: a modified migration fails" "^1_init.sql$" in_backend scripts/check-migrations-immutable.sh HEAD
expect fail "migrations: a modified migration fails with GIT_DIR" "^1_init.sql$" with_git_dir scripts/check-migrations-immutable.sh HEAD
expect fail "migrations: an unknown base ref fails" "no-such-ref" in_backend scripts/check-migrations-immutable.sh no-such-ref

expect pass "openapi: an unchanged contract passes" in_backend scripts/check-openapi-breaking.sh "$oasdiff" HEAD
sed -i.bak -e 's/required: \[id, name\]/required: [id]/' -e '/name: {type: string}/d' "$repo/backend/api/openapi.yaml"
expect fail "openapi: removing a response property fails" "response-property-removed|removed the .*property" in_backend scripts/check-openapi-breaking.sh "$oasdiff" HEAD
expect fail "openapi: removing a response property fails with GIT_DIR" "response-property-removed|removed the .*property" with_git_dir scripts/check-openapi-breaking.sh "$oasdiff" HEAD

# golang.org/x/text before v0.3.7: language.Parse has GO-2021-0113.
vuln=$tmp/vuln
mkdir -p "$vuln"
cat >"$vuln/main.go" <<'EOF'
package main

import "golang.org/x/text/language"

func main() { _, _ = language.Parse("en") }
EOF
(cd "$vuln" && go mod init vulnfixture >/dev/null 2>&1 && go get golang.org/x/text@v0.3.6 >/dev/null 2>&1)
vulncheck() { (cd "$vuln" && "$govulncheck" ./...); }
expect fail "govulncheck: calling a known-vulnerable function fails" "GO-2021-0113" vulncheck

if ((failures > 0)); then
  echo "$failures self-test case(s) failed" >&2
  exit 1
fi
