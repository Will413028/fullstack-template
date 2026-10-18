#!/usr/bin/env bash
# One-off check for the Go rewrite (removed with the plan at close-out): every business behavior of the old FastAPI
# backend has a matching result in the Go backend. The contract changed on purpose, so shapes are asserted per side
# and only business results are compared: success or failure, item contents and owner, which items a user sees.
#
# Old: tag archive/pre-go-rewrite in a temporary worktree, uv + alembic + uvicorn on the host, own PostgreSQL 18.
# New: this checkout's backend/Dockerfile image, roles from deploy/postgres/initdb, `app migrate`, then `app serve`,
#      own PostgreSQL 18 on its own Docker network. Neither side reads .env.
#
# Usage: bash parity.sh            (needs docker, uv, git, curl, jq, openssl, python3)
# Exit 0 when every row passes; 1 when any row fails; 2 when the environment could not be set up.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(git -C "$HERE" rev-parse --show-toplevel)"
OLD_REF="${PARITY_OLD_REF:-archive/pre-go-rewrite}"
ID="parity-$$-$RANDOM"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/$ID.XXXXXX")"
OLD_PID=""

cleanup() {
  [ -n "$OLD_PID" ] && kill "$OLD_PID" 2>/dev/null || true
  # -v: postgres:18 declares a VOLUME, which would otherwise outlive every run as an anonymous volume.
  docker rm -f -v "$ID-old-pg" "$ID-new-pg" "$ID-new-api" >/dev/null 2>&1 || true
  docker network rm "$ID-net" >/dev/null 2>&1 || true
  docker image rm "$ID-backend" >/dev/null 2>&1 || true
  git -C "$REPO" worktree remove --force "$WORK/old" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT
# A child such as `docker exec` may handle SIGINT itself, and bash would then carry on; stop here instead.
trap 'exit 130' INT
trap 'exit 143' TERM

die() { echo "parity.sh: $*" >&2; exit 2; }

free_port() { python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

# wait_pg CONTAINER: TCP readiness, so the image's socket-only init server does not count as ready.
wait_pg() {
  for _ in $(seq 60); do
    docker exec "$1" pg_isready -h 127.0.0.1 -q 2>/dev/null && return 0
    sleep 1
  done
  docker logs "$1" >&2
  die "$1 not ready"
}

# wait_http URL LOG: until URL answers 200.
wait_http() {
  for _ in $(seq 90); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "$1" || true)" = 200 ] && return 0
    sleep 1
  done
  cat "$2" >&2 2>/dev/null || true
  die "$1 not ready"
}

# ---------------------------------------------------------------------------------------------------------------
# Old system
echo "== old: $OLD_REF"
git -C "$REPO" worktree add --detach "$WORK/old" "$OLD_REF" >/dev/null 2>&1 || die "worktree add $OLD_REF failed"
docker run -d --name "$ID-old-pg" -e POSTGRES_PASSWORD=parity -e POSTGRES_DB=parity \
  -p 127.0.0.1::5432 postgres:18 >/dev/null || die "old postgres did not start"
wait_pg "$ID-old-pg"
OLD_PG_PORT="$(docker port "$ID-old-pg" 5432/tcp | head -1 | sed 's/.*://')"
OLD_PORT="$(free_port)"
# env -i: only these settings reach the old app, not CORS_ORIGINS, COOKIE_SAMESITE or others from this shell.
(
  cd "$WORK/old/backend"
  exec env -i HOME="$HOME" PATH="$PATH" \
    DATABASE_URL="postgresql://postgres:parity@127.0.0.1:$OLD_PG_PORT/parity" \
    SECRET_KEY="parity-only-secret-key-0123456789abcdef" COOKIE_SECURE=false MODE=dev \
    bash -c "set -e
uv sync --frozen --quiet
uv run --frozen alembic upgrade head
# Rate limit off the same way tests/conftest.py does.
exec uv run --frozen python -c \"from src.auth.router import limiter; limiter.enabled = False; import uvicorn; \
from src.main import app; uvicorn.run(app, host='127.0.0.1', port=$OLD_PORT, log_level='warning')\""
) >"$WORK/old.log" 2>&1 &
OLD_PID=$!
wait_http "http://127.0.0.1:$OLD_PORT/health" "$WORK/old.log"
OLD="http://127.0.0.1:$OLD_PORT"

# ---------------------------------------------------------------------------------------------------------------
# New system
echo "== new: $(git -C "$REPO" rev-parse --short HEAD) plus working tree changes"
mkdir "$WORK/secrets"
for f in postgres_password db_owner_password db_app_password; do
  openssl rand -hex 16 >"$WORK/secrets/$f"
done
chmod 0755 "$WORK/secrets"
chmod 0644 "$WORK/secrets"/*
docker network create "$ID-net" >/dev/null || die "network create failed"
docker run -d --name "$ID-new-pg" --network "$ID-net" \
  -e POSTGRES_PASSWORD_FILE=/run/secrets/postgres_password -e POSTGRES_DB=parity \
  -e DB_OWNER_USER=parity_owner -e DB_APP_USER=parity_app \
  -v "$WORK/secrets:/run/secrets:ro" -v "$REPO/deploy/postgres/initdb:/docker-entrypoint-initdb.d:ro" \
  postgres:18 >/dev/null || die "new postgres did not start"
docker build -q -t "$ID-backend" "$REPO/backend" >/dev/null || die "docker build failed"
wait_pg "$ID-new-pg"
docker run --rm --network "$ID-net" -v "$WORK/secrets:/run/secrets:ro" -e MODE=dev \
  -e MIGRATE_DATABASE_URL="postgres://parity_owner@$ID-new-pg:5432/parity?sslmode=disable" \
  -e MIGRATE_DATABASE_PASSWORD_FILE=/run/secrets/db_owner_password -e DB_APP_USER=parity_app \
  "$ID-backend" migrate >"$WORK/new-migrate.log" 2>&1 || { cat "$WORK/new-migrate.log" >&2; die "migrate failed"; }
docker run -d --name "$ID-new-api" --network "$ID-net" -p 127.0.0.1::8000 -v "$WORK/secrets:/run/secrets:ro" \
  -e MODE=dev -e DATABASE_URL="postgres://parity_app@$ID-new-pg:5432/parity?sslmode=disable" \
  -e DATABASE_PASSWORD_FILE=/run/secrets/db_app_password -e COOKIE_SECURE=false -e AUTH_RATE_LIMIT_PER_MIN=10000 \
  "$ID-backend" serve >/dev/null || die "serve did not start"
NEW_PORT="$(docker port "$ID-new-api" 8000/tcp | head -1 | sed 's/.*://')"
wait_http "http://127.0.0.1:$NEW_PORT/health/ready" /dev/null
NEW="http://127.0.0.1:$NEW_PORT/api"

# ---------------------------------------------------------------------------------------------------------------
# Helpers

# call SIDE USER METHOD PATH [BODY [CONTENT_TYPE]]: sets ST (status) and BODY; response headers in $WORK/headers.
# USER names a cookie jar per side; "anon" sends no cookies.
call() {
  local side=$1 user=$2 method=$3 path=$4 data=${5-} ctype=${6-application/json} base
  if [ "$side" = old ]; then base=$OLD; else base=$NEW; fi
  local args=(-sS -o "$WORK/body" -D "$WORK/headers" -w '%{http_code}' -X "$method")
  if [ "$user" != anon ]; then
    touch "$WORK/jar-$side-$user"
    args+=(-b "$WORK/jar-$side-$user" -c "$WORK/jar-$side-$user")
  fi
  if [ -n "$data" ]; then args+=(-H "Content-Type: $ctype" --data-binary "$data"); fi
  # A server that died answers 000 and fails the row instead of ending the script with curl's exit code.
  : >"$WORK/body"
  : >"$WORK/headers"
  ST="$(curl "${args[@]}" "$base$path" 2>/dev/null || true)"
  BODY="$(cat "$WORK/body")"
}
j() { jq -r "$1" <<<"$BODY" 2>/dev/null || echo "<not json: ${BODY:0:80}>"; }
header() { tr -d '\r' <"$WORK/headers" | awk -v h="$(tr '[:upper:]' '[:lower:]' <<<"$1")" -F': ' 'tolower($1) == h { print $2 }'; }
cookie_set() { tr -d '\r' <"$WORK/headers" | grep -ci "^set-cookie: $1=[^;]" || true; }

FAILED=()
ROW_FAILS=0
row() { ROW=$1; ROW_FAILS=0; }
eq() {
  if [ "$2" != "$3" ]; then
    printf '      %s: got %q, want %q\n' "$1" "$2" "$3"
    ROW_FAILS=$((ROW_FAILS + 1))
  fi
}
end_row() {
  if ((ROW_FAILS)); then echo "FAIL  $ROW"; FAILED+=("$ROW"); else echo "ok    $ROW"; fi
}

# titles SIDE USER: every page of the user's item list at page size 2, as sorted "title|description" lines in
# TITLES and the item count of each page in SIZES (e.g. "2 2 1"). The old list has no ORDER BY, so only the set is
# compared; a line seen twice fails the row, and so does a list that has not ended after 10 pages.
titles() {
  local side=$1 user=$2 out="" page=1 cursor="" field ended=0
  SIZES=""
  if [ "$side" = old ]; then field=.data; else field=.items; fi
  while [ "$page" -le 10 ]; do
    if [ "$side" = old ]; then
      call old "$user" GET "/items?page=$page&size=2"
    else
      call new "$user" GET "/items?limit=2${cursor:+&cursor=$cursor}"
    fi
    eq "$side list page $page status" "$ST" 200
    out+="$(j "${field}[] | \"\(.title)|\(.description)\"")"$'\n'
    SIZES+="${SIZES:+ }$(j "$field | length")"
    if [ "$side" = old ]; then
      [ "$page" -ge "$(j '.pages')" ] && ended=1
    else
      cursor="$(j '.next_cursor // empty')"
      [ -z "$cursor" ] && ended=1
    fi
    ((ended)) && break
    page=$((page + 1))
  done
  eq "$side list ends within 10 pages" "$ended" 1
  TITLES="$(grep -v '^$' <<<"$out" | sort || true)"
  eq "$side list has no duplicates" "$(uniq -d <<<"$TITLES")" ""
}

A="parity_a_$RANDOM"
# shellcheck disable=SC2034 # read through ${!u} in the registration row
B="parity_b_$RANDOM"
# Test passwords are made per run from 8 random hex characters, so no literal password sits in the repo.
R="$(openssl rand -hex 4)"
PW="Parity-${R}-Pass-1" # 22 characters with a digit, valid under both rules
WEAK_OLD="S${R:0:4}1a"  # 7 characters: too short for the old rules
WEAK_NEW="A${R}12345"   # 14 characters: one short of the new minimum
LOWER="parity${R}lower1" # 20 characters, no uppercase: the old rules refuse it, the new accept it
WRONG="Wrong-${R}-pass-x"

echo
echo "== rows (old request and expectation -> new request and expectation)"

# ---------------------------------------------------------------------------------------------------------------
row "註冊: POST /auth/register 200 {data:User}+cookies -> POST /api/users 201 User+Location+session cookie"
for u in A B; do
  acct=${!u}
  call old "$u" POST /auth/register "{\"account\":\"$acct\",\"password\":\"$PW\"}"
  eq "old $u status" "$ST" 200
  eq "old $u account" "$(j .data.account)" "$acct"
  eq "old $u access cookie" "$(cookie_set access_token)" 1
  OLD_ACCT=$(j .data.account)
  call new "$u" POST /users "{\"account\":\"$acct\",\"password\":\"$PW\"}"
  eq "new $u status" "$ST" 201
  eq "new $u Location" "$(header Location)" "/api/users/$(j .id)"
  eq "new $u session cookie" "$(cookie_set session)" 1
  eq "same $u account" "$(j .account)" "$OLD_ACCT"
done
end_row

row "重複註冊: 409 {detail} -> 409 problem conflict/account_taken"
call old anon POST /auth/register "{\"account\":\"$A\",\"password\":\"$PW\"}"
eq "old status" "$ST" 409
call new anon POST /users "{\"account\":\"$A\",\"password\":\"$PW\"}"
eq "new status" "$ST" 409
eq "new type" "$(j .type)" /problems/conflict
eq "new code" "$(j .code)" account_taken
end_row

row "弱密碼（各依自己的規則）: 7 字 422 {errors.password} -> 14 字 422 /password too_short；帳號都沒建立"
call old anon POST /auth/register "{\"account\":\"${A}_weak\",\"password\":\"$WEAK_OLD\"}"
eq "old status" "$ST" 422
eq "old field" "$(j '.errors | has("password")')" true
call old anon POST /auth/login "username=${A}_weak&password=$WEAK_OLD" application/x-www-form-urlencoded
eq "old not created" "$ST" 401
call new anon POST /users "{\"account\":\"${A}_weak\",\"password\":\"$WEAK_NEW\"}"
eq "new status" "$ST" 422
eq "new pointer" "$(j '.errors[0].pointer')" /password
eq "new code" "$(j '.errors[0].code')" too_short
call new anon POST /sessions "{\"account\":\"${A}_weak\",\"password\":\"$WEAK_NEW\"}"
eq "new not created" "$ST" 401
end_row

row "刻意差異 B12（NIST 不設組合規則）: 全小寫 20 字 422 -> 201"
call old anon POST /auth/register "{\"account\":\"${A}_lower\",\"password\":\"$LOWER\"}"
eq "old status" "$ST" 422
call new anon POST /users "{\"account\":\"${A}_lower\",\"password\":\"$LOWER\"}"
eq "new status" "$ST" 201
end_row

row "登入成功: POST /auth/login form 200 {data:User} -> POST /api/sessions JSON 201 Session+Location；cookie 可用"
call old A-login POST /auth/login "username=$A&password=$PW" application/x-www-form-urlencoded
eq "old status" "$ST" 200
OLD_ACCT=$(j .data.account)
eq "old account" "$OLD_ACCT" "$A"
call new A-login POST /sessions "{\"account\":\"$A\",\"password\":\"$PW\"}"
eq "new status" "$ST" 201
eq "new current" "$(j .current)" true
eq "new Location" "$(header Location)" "/api/sessions/$(j .id)"
eq "same account" "$(j .user.account)" "$OLD_ACCT"
call old A-login GET /auth/me
eq "old login cookie works" "$ST" 200
call new A-login GET /users/me
eq "new login cookie works" "$ST" 200
end_row

row "登入失敗（密碼錯、帳號不存在）: 401 {detail} -> 401 problem unauthenticated/invalid_credentials"
for acct in "$A" "nobody_$RANDOM"; do
  call old anon POST /auth/login "username=$acct&password=$WRONG" application/x-www-form-urlencoded
  eq "old $acct status" "$ST" 401
  call new anon POST /sessions "{\"account\":\"$acct\",\"password\":\"$WRONG\"}"
  eq "new $acct status" "$ST" 401
  eq "new $acct type" "$(j .type)" /problems/unauthenticated
  eq "new $acct code" "$(j .code)" invalid_credentials
done
end_row

row "目前使用者: GET /auth/me 200 {data:User} -> GET /api/users/me 200 User"
call old A GET /auth/me
eq "old status" "$ST" 200
OLD_A_ID=$(j .data.id)
OLD_ME="$(j '"\(.data.account)|\(.data.is_disabled)"')"
call new A GET /users/me
eq "new status" "$ST" 200
NEW_A_ID=$(j .id)
eq "new role" "$(j .role)" user
eq "same account and is_disabled" "$(j '"\(.account)|\(.is_disabled)"')" "$OLD_ME"
end_row

row "未登入: GET /auth/me、GET /items、POST /items 401 -> GET /api/users/me、GET/POST /api/items 401 problem"
for req in "GET /auth/me:GET /users/me" "GET /items:GET /items" "POST /items:POST /items"; do
  o=${req%%:*} n=${req#*:}
  body=""
  [ "${o% *}" = POST ] && body='{"title":"x"}'
  call old anon "${o% *}" "${o#* }" "$body"
  eq "old $o" "$ST" 401
  call new anon "${n% *}" "${n#* }" "$body"
  eq "new $n" "$ST" 401
  eq "new $n type" "$(j .type)" /problems/unauthenticated
done
end_row

row "item 建立: POST /items 200 {data:Item} -> POST /api/items 201 Item+Location；owner 是建立者"
OLD_IDS=()
NEW_IDS=()
for i in 1 2 3 4 5; do
  if [ "$i" = 3 ]; then body="{\"title\":\"parity-$i\"}"; else body="{\"title\":\"parity-$i\",\"description\":\"desc-$i\"}"; fi
  call old A POST /items "$body"
  eq "old $i status" "$ST" 200
  eq "old $i owner" "$(j .data.owner_id)" "$OLD_A_ID"
  OLD_IDS+=("$(j .data.id)")
  OLD_ITEM="$(j '"\(.data.title)|\(.data.description)"')"
  call new A POST /items "$body"
  eq "new $i status" "$ST" 201
  eq "new $i owner" "$(j .owner_id)" "$NEW_A_ID"
  eq "new $i Location" "$(header Location)" "/api/items/$(j .id)"
  NEW_IDS+=("$(j .id)")
  eq "same $i title and description" "$(j '"\(.title)|\(.description)"')" "$OLD_ITEM"
done
call old B POST /items '{"title":"parity-b","description":"of b"}'
eq "old b status" "$ST" 200
call new B POST /items '{"title":"parity-b","description":"of b"}'
eq "new b status" "$ST" 201
end_row

row "item 讀取: GET /items/{id} 200 {data:Item} -> GET /api/items/{id} 200 Item"
call old A GET "/items/${OLD_IDS[0]}"
eq "old status" "$ST" 200
OLD_ITEM="$(j "\"\(.data.title)|\(.data.description)|\(.data.owner_id == $OLD_A_ID)\"")"
call new A GET "/items/${NEW_IDS[0]}"
eq "new status" "$ST" 200
eq "same item" "$(j "\"\(.title)|\(.description)|\(.owner_id == \"$NEW_A_ID\")\"")" "$OLD_ITEM"
eq "content" "$OLD_ITEM" "parity-1|desc-1|true"
end_row

row "列出與分頁（每頁 2）: page/size 3 頁 -> limit/cursor 3 頁；同一組 item、只有自己的、不重複"
EXPECTED="$(printf '%s\n' 'parity-1|desc-1' 'parity-2|desc-2' 'parity-3|null' 'parity-4|desc-4' 'parity-5|desc-5' | sort)"
titles old A
OLD_TITLES=$TITLES
OLD_SIZES=$SIZES
titles new A
eq "same items" "$TITLES" "$OLD_TITLES"
eq "same page sizes" "$SIZES" "$OLD_SIZES"
eq "old items" "$OLD_TITLES" "$EXPECTED"
eq "old page sizes" "$OLD_SIZES" "2 2 1"
titles old B
eq "old b sees only own" "$TITLES" "parity-b|of b"
titles new B
eq "new b sees only own" "$TITLES" "parity-b|of b"
end_row

row "item 修改: PATCH JSON 200 -> PATCH merge-patch 200；沒送的 description 保留"
call old A PATCH "/items/${OLD_IDS[1]}" '{"title":"parity-2-renamed"}'
eq "old status" "$ST" 200
OLD_ITEM="$(j '"\(.data.title)|\(.data.description)"')"
call new A PATCH "/items/${NEW_IDS[1]}" '{"title":"parity-2-renamed"}' application/merge-patch+json
eq "new status" "$ST" 200
eq "same result" "$(j '"\(.title)|\(.description)"')" "$OLD_ITEM"
eq "content" "$OLD_ITEM" "parity-2-renamed|desc-2"
call old A GET "/items/${OLD_IDS[1]}"
OLD_ITEM="$(j '"\(.data.title)|\(.data.description)"')"
call new A GET "/items/${NEW_IDS[1]}"
eq "same persisted" "$(j '"\(.title)|\(.description)"')" "$OLD_ITEM"
end_row

row "item 刪除: DELETE 200 {detail} -> DELETE 204；之後讀取 404，列表少一筆"
call old A DELETE "/items/${OLD_IDS[4]}"
eq "old status" "$ST" 200
call new A DELETE "/items/${NEW_IDS[4]}"
eq "new status" "$ST" 204
eq "new empty body" "$BODY" ""
call old A GET "/items/${OLD_IDS[4]}"
eq "old get after delete" "$ST" 404
call new A GET "/items/${NEW_IDS[4]}"
eq "new get after delete" "$ST" 404
titles old A
OLD_TITLES=$TITLES
OLD_SIZES=$SIZES
titles new A
eq "same items" "$TITLES" "$OLD_TITLES"
eq "same page sizes" "$SIZES" "$OLD_SIZES"
eq "old page sizes" "$OLD_SIZES" "2 2"
eq "items" "$OLD_TITLES" "$(grep -v '^parity-5|' <<<"$EXPECTED" | sed 's/^parity-2|/parity-2-renamed|/' | sort)"
end_row

row "刻意差異 E1（讀取別人的 item）: 200 -> 404 problem not-found"
call old B GET "/items/${OLD_IDS[0]}"
eq "old status (defect E1)" "$ST" 200
call new B GET "/items/${NEW_IDS[0]}"
eq "new status" "$ST" 404
eq "new type" "$(j .type)" /problems/not-found
end_row

row "修改、刪除別人的 item: 403 -> 404（A11）；兩邊的 item 都沒變"
call old B PATCH "/items/${OLD_IDS[0]}" '{"title":"hijacked"}'
eq "old patch" "$ST" 403
call old B DELETE "/items/${OLD_IDS[0]}"
eq "old delete" "$ST" 403
call new B PATCH "/items/${NEW_IDS[0]}" '{"title":"hijacked"}' application/merge-patch+json
eq "new patch" "$ST" 404
call new B DELETE "/items/${NEW_IDS[0]}"
eq "new delete" "$ST" 404
call old A GET "/items/${OLD_IDS[0]}"
OLD_ITEM="$(j '"\(.data.title)|\(.data.description)"')"
call new A GET "/items/${NEW_IDS[0]}"
eq "same item" "$(j '"\(.title)|\(.description)"')" "$OLD_ITEM"
eq "unchanged" "$OLD_ITEM" "parity-1|desc-1"
end_row

echo
if ((${#FAILED[@]})); then
  echo "parity.sh: ${#FAILED[@]} row(s) failed"
  exit 1
fi
echo "parity.sh: all rows passed"
