#!/usr/bin/env bash
# Shows that the checks make check relies on still fail when they should. Each case builds what it needs in a temporary
# directory, so nothing here is tracked (a vulnerable fixture module in the repository would fail osv-scanner).
#
#   selftest-checks.sh <buf command>
#
# govulncheck is built from tools/go.mod and buf is the root Makefile's pin, the versions make check runs. A case that
# must fail passes only when the output names the expected reason, so a tool that did not start cannot count as a
# detection.
#
# - check-migrations-immutable.sh: a modified existing migration fails, also with GIT_DIR exported as in a git hook;
#   a base without migrations passes; an unknown base ref fails instead of passing as "nothing to compare".
# - buf lint with api/buf.yaml: a service without the Service suffix fails (a STANDARD rule that BASIC lacks).
# - check-buf-breaking.sh with api/buf.yaml, against a v1 package: deleting a field, an RPC, a service or a whole
#   package fails, also with GIT_DIR; a base without api/buf.yaml passes.
# - govulncheck: a module that calls a known-vulnerable function fails.
# - deadcode.sh: a function nothing calls fails.
# - squawk.sh: dropping a constraint the previous release may rely on fails (ban-drop-constraint), with the squawk
#   tools/package.json pins.
set -euo pipefail
buf_arg=${1:?usage: selftest-checks.sh <buf command>}
buf=$(cd "$(dirname "$buf_arg")" && pwd)/$(basename "$buf_arg")
cd "$(dirname "$0")/.."
unset GIT_DIR GIT_WORK_TREE

scripts=$PWD/scripts
root=$(cd .. && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failures=0
go build -modfile=tools/go.mod -o "$tmp/bin/" golang.org/x/vuln/cmd/govulncheck golang.org/x/tools/cmd/deadcode
govulncheck=$tmp/bin/govulncheck
deadcode=$tmp/bin/deadcode

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

# A repository shaped like this one: the migrations live under backend/, the contract and its scripts at the root.
repo=$tmp/repo
mkdir -p "$repo/backend/scripts" "$repo/backend/migrations" "$repo/scripts" "$repo/api/app/selftest/v1" \
  "$repo/api/app/gone/v1"
cp "$scripts/check-migrations-immutable.sh" "$repo/backend/scripts/"
cp "$root/scripts/check-buf-breaking.sh" "$repo/scripts/"
git -C "$repo" init -q -b main
git -C "$repo" -c user.name=t -c user.email=t@t commit -q --allow-empty -m empty
in_backend() { (cd "$repo/backend" && "$@"); }
with_git_dir() { (cd "$repo/backend" && GIT_DIR="$repo/.git" "$@"); }
breaking() { (cd "$repo/backend" && "$repo/scripts/check-buf-breaking.sh" "$buf" HEAD); }
breaking_git_dir() { (cd "$repo/backend" && GIT_DIR="$repo/.git" "$repo/scripts/check-buf-breaking.sh" "$buf" HEAD); }

expect pass "migrations: a base without migrations passes" in_backend scripts/check-migrations-immutable.sh HEAD
expect pass "buf breaking: a base without api/buf.yaml passes" breaking

printf 'CREATE TABLE t (id int);\n' >"$repo/backend/migrations/1_init.sql"
printf 'h1:dir\n1_init.sql h1:file\n' >"$repo/backend/migrations/atlas.sum"
# The real api/buf.yaml with a released v1 package: a field, two RPCs, and a second package for the cases to delete.
cp "$root/api/buf.yaml" "$repo/api/"
proto=$repo/api/app/selftest/v1/selftest.proto
cat >"$proto" <<'EOF'
syntax = "proto3";

package app.selftest.v1;

message Thing {
  string id = 1;
  string name = 2;
}

message GetThingRequest {
  string id = 1;
}

message GetThingResponse {
  Thing thing = 1;
}

message DeleteThingRequest {
  string id = 1;
}

message DeleteThingResponse {}

service ThingAppService {
  rpc GetThing(GetThingRequest) returns (GetThingResponse);
  rpc DeleteThing(DeleteThingRequest) returns (DeleteThingResponse);
}

message ListThingsRequest {}

message ListThingsResponse {
  repeated Thing things = 1;
}

service ThingAdminService {
  rpc ListThings(ListThingsRequest) returns (ListThingsResponse);
}
EOF
cat >"$repo/api/app/gone/v1/gone.proto" <<'EOF'
syntax = "proto3";

package app.gone.v1;

message Note {
  string id = 1;
}
EOF
cp "$proto" "$tmp/selftest.proto"
git -C "$repo" add -A
git -C "$repo" -c user.name=t -c user.email=t@t commit -q -m base

expect pass "migrations: unchanged migrations pass" in_backend scripts/check-migrations-immutable.sh HEAD
expect pass "migrations: unchanged migrations pass with GIT_DIR" with_git_dir scripts/check-migrations-immutable.sh HEAD
printf -- '-- edited\n' >>"$repo/backend/migrations/1_init.sql"
expect fail "migrations: a modified migration fails" "^1_init.sql$" in_backend scripts/check-migrations-immutable.sh HEAD
expect fail "migrations: a modified migration fails with GIT_DIR" "^1_init.sql$" with_git_dir scripts/check-migrations-immutable.sh HEAD
expect fail "migrations: an unknown base ref fails" "no-such-ref" in_backend scripts/check-migrations-immutable.sh no-such-ref

expect pass "buf lint: the fixture contract passes" "$buf" lint "$repo/api"
expect pass "buf breaking: an unchanged contract passes" breaking
sed -i.bak -e '/string name = 2;/d' "$proto"
expect fail "buf breaking: deleting a field fails" 'field "2" with name "name" on message "Thing" was deleted' breaking
expect fail "buf breaking: deleting a field fails with GIT_DIR" 'field "2" with name "name" on message "Thing" was deleted' breaking_git_dir
cp "$tmp/selftest.proto" "$proto"
sed -i.bak -e '/rpc DeleteThing/d' "$proto"
expect fail "buf breaking: deleting an RPC fails" 'RPC "DeleteThing" on service "ThingAppService" was deleted' breaking
cp "$tmp/selftest.proto" "$proto"
sed -i.bak -e '/^service ThingAdminService/,/^}/d' "$proto"
expect fail "buf breaking: deleting a service fails" 'service "ThingAdminService" was deleted' breaking
cp "$tmp/selftest.proto" "$proto"
mv "$repo/api/app/gone" "$tmp/gone"
expect fail "buf breaking: deleting a package fails" 'package "app.gone.v1" was deleted' breaking
mv "$tmp/gone" "$repo/api/app/gone"
sed -i.bak -e 's/service ThingAppService/service ThingApp/' "$proto"
expect fail "buf lint: a service without the Service suffix fails" 'should be suffixed with "Service"' "$buf" lint "$repo/api"
cp "$tmp/selftest.proto" "$proto"
rm -f "$proto.bak"

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

dead=$tmp/dead
mkdir -p "$dead"
printf 'package main\n\nfunc main() { used() }\n\nfunc used() {}\n' >"$dead/main.go"
(cd "$dead" && go mod init deadfixture >/dev/null 2>&1)
deadcheck() { (cd "$dead" && "$scripts/deadcode.sh" "$deadcode"); }
expect pass "deadcode: a module whose functions are all called passes" deadcheck
printf '\nfunc unused() {}\n' >>"$dead/main.go"
expect fail "deadcode: a function nothing calls fails" "unreachable func: unused" deadcheck

squawk_dir=$tmp/squawk
mkdir -p "$squawk_dir/scripts" "$squawk_dir/migrations"
cp "$scripts/squawk.sh" "$squawk_dir/scripts/"
cp .squawk.toml "$squawk_dir/"
printf 'ALTER TABLE "t" DROP CONSTRAINT "t_pkey", ADD PRIMARY KEY ("a", "b");\n' >"$squawk_dir/migrations/1_pk.sql"
squawkcheck() { "$squawk_dir/scripts/squawk.sh" "$PWD/tools/node_modules/.bin/squawk"; }
expect fail "squawk: dropping a constraint the previous release relies on fails" "ban-drop-constraint" squawkcheck

if ((failures > 0)); then
  echo "$failures self-test case(s) failed" >&2
  exit 1
fi
