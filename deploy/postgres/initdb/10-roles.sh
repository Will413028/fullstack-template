#!/usr/bin/env bash
# Runs once, when the postgres image initializes an empty volume (docker-entrypoint-initdb.d), and in the Go test
# harness (internal/platform/dbtest). Creates the two login roles (docs/adr/2026-10-18-go-backend.md D5):
# - DB_OWNER_USER owns the database and the public schema; only `app migrate` uses it. No superuser, no CREATEROLE.
# - DB_APP_USER is what serve connects as: CONNECT here, DML granted by `app migrate` on every run.
# The superuser (POSTGRES_USER) is for initialization and maintenance from inside the container only.
# Passwords come from the compose secrets mounted under /run/secrets.
set -euo pipefail
password() { tr -d '\r\n' < "/run/secrets/$1"; }
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v db="$POSTGRES_DB" -v owner="${DB_OWNER_USER:?}" -v app="${DB_APP_USER:?}" \
  -v owner_pw="$(password db_owner_password)" -v app_pw="$(password db_app_password)" <<'SQL'
CREATE ROLE :"owner" LOGIN PASSWORD :'owner_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE :"app" LOGIN PASSWORD :'app_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE;
ALTER DATABASE :"db" OWNER TO :"owner";
ALTER SCHEMA public OWNER TO :"owner";
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db" TO :"app";
SQL
