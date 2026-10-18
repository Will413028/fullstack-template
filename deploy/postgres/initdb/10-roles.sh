#!/usr/bin/env bash
# Runs once, when the postgres image initializes an empty volume (docker-entrypoint-initdb.d), and in the Go test
# harness (internal/platform/dbtest). Creates the two login roles (docs/adr/2026-10-18-go-backend.md D5):
# - DB_OWNER_USER owns the database and the public schema; only `app migrate` uses it. No superuser, no CREATEROLE.
# - DB_APP_USER is what serve connects as: CONNECT here, DML granted by `app migrate` on every run.
# The superuser (POSTGRES_USER) is for initialization and maintenance from inside the container only.
# Passwords come from the compose secrets mounted under /run/secrets and reach psql through its environment, not
# its command line. A missing or empty file fails initialization instead of creating a role without a password.
set -euo pipefail
password() {
  local v
  v="$(tr -d '\r\n' < "/run/secrets/$1")"
  if [ -z "$v" ]; then echo "10-roles.sh: /run/secrets/$1 is empty" >&2; exit 1; fi
  printf '%s' "$v"
}
OWNER_PW="$(password db_owner_password)"
APP_PW="$(password db_app_password)"
export OWNER_PW APP_PW
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v db="$POSTGRES_DB" -v owner="${DB_OWNER_USER:?}" -v app="${DB_APP_USER:?}" <<'SQL'
\getenv owner_pw OWNER_PW
\getenv app_pw APP_PW
CREATE ROLE :"owner" LOGIN PASSWORD :'owner_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE :"app" LOGIN PASSWORD :'app_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE;
ALTER DATABASE :"db" OWNER TO :"owner";
ALTER SCHEMA public OWNER TO :"owner";
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db" TO :"app";
SQL
