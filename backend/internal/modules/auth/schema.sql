-- auth module: desired state of its tables. Atlas generates migrations from this file (make migration); never edit
-- applied migrations. Type rules follow docs/adr/2026-10-18-go-backend.md (UUIDv7 keys, text with CHECK, timestamptz).

-- Accounts are stored lowercased, so uniqueness ignores case; the contract allows [A-Za-z0-9_.-]{3,50}.
CREATE TABLE users (
  id            uuid        PRIMARY KEY DEFAULT uuidv7(),
  account       text        NOT NULL UNIQUE
                            CHECK (account = lower(account) AND char_length(account) BETWEEN 3 AND 50),
  password_hash text        NOT NULL,
  role          text        NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
  is_disabled   boolean     NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Opaque sessions: the cookie holds a random token, the table only its SHA-256. Every request looks a session up by
-- token_hash; id is the public identifier the API uses for the session resource, so nothing derived from the token
-- leaves the server.
CREATE TABLE sessions (
  token_hash   bytea       PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  id           uuid        NOT NULL DEFAULT uuidv7(),
  user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at   timestamptz NOT NULL,
  last_used_at timestamptz NOT NULL,
  expires_at   timestamptz NOT NULL
);

CREATE UNIQUE INDEX sessions_id_key ON sessions (id);
-- A user's sessions newest first (keyset on id), and deleting them all.
CREATE INDEX sessions_user_id_id_idx ON sessions (user_id, id DESC);

-- Replaced by sign_in_throttles; nothing reads or writes it any more. It stays for one release so the previous
-- backend, which still writes it with ON CONFLICT (account_key), keeps working while a deploy or a rollback runs
-- (expand and contract, docs/rules/database-design.md); the next release drops it.
CREATE TABLE login_throttles (
  account_key     text        PRIMARY KEY,
  failures        bigint      NOT NULL DEFAULT 0 CHECK (failures >= 0),
  next_allowed_at timestamptz,
  updated_at      timestamptz NOT NULL
);

-- Sign-in throttling, keyed by the normalized account name whether or not the account exists, and by the client the
-- attempt came from: client_key is 'ip:<address, or its /64 for IPv6>' or 'device:<trusted device id>'. Each pair
-- has its own backoff, so one client's failures do not lock out another. The row with client_key '' counts failures
-- from untrusted clients across every address: from 100 on it refuses them (NIST SP 800-63B 3.2.2) until a sign-in
-- succeeds, an admin unlocks the account, or 24 h pass without attempts. A trusted device is never refused by it.
CREATE TABLE sign_in_throttles (
  account_key     text        NOT NULL,
  client_key      text        NOT NULL,
  failures        bigint      NOT NULL DEFAULT 0 CHECK (failures >= 0),
  next_allowed_at timestamptz,
  updated_at      timestamptz NOT NULL,
  PRIMARY KEY (account_key, client_key)
);

-- Browsers that have signed in to the account (OWASP "Slow Down Online Guessing Attacks with Device Cookies"): the
-- device cookie holds a random token, the table only its SHA-256, as for sessions. Deleted when the user signs out
-- everywhere, is disabled, recovered or deleted, and once expired.
CREATE TABLE trusted_devices (
  token_hash bytea       PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  id         uuid        NOT NULL DEFAULT uuidv7(),
  user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX trusted_devices_id_key ON trusted_devices (id);
CREATE INDEX trusted_devices_user_id_idx ON trusted_devices (user_id);

-- Who did what to which account, written in the transaction that made the change. The comment makes the table
-- append-only: app migrate gives the app role INSERT and SELECT but not UPDATE or DELETE here, so the application
-- cannot rewrite the record, and deletes rows older than the retention period (internal/platform/migrate).
-- actor_id and target_id have no foreign key, so the record outlives the accounts; a NULL actor is the operator
-- on the host (recover-admin).
CREATE TABLE audit_events (
  id         uuid        PRIMARY KEY DEFAULT uuidv7(),
  at         timestamptz NOT NULL,
  actor_id   uuid,
  action     text        NOT NULL
                         CHECK (action IN ('user.disabled', 'user.enabled', 'user.deleted', 'user.unlocked', 'admin.recovered')),
  target_id  uuid        NOT NULL,
  request_id text
);
COMMENT ON TABLE audit_events IS 'append-only: retain 365 days by at';

-- Retention deletes by age.
CREATE INDEX audit_events_at_idx ON audit_events (at);
