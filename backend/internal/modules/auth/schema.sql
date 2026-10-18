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

-- Opaque sessions: the cookie holds a random token, the table only its SHA-256.
CREATE TABLE sessions (
  token_hash   bytea       PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at   timestamptz NOT NULL,
  last_used_at timestamptz NOT NULL,
  expires_at   timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- Per-account login throttling, keyed by the normalized account name whether or not the account exists.
CREATE TABLE login_throttles (
  account_key     text        PRIMARY KEY,
  failures        bigint      NOT NULL DEFAULT 0 CHECK (failures >= 0),
  next_allowed_at timestamptz,
  updated_at      timestamptz NOT NULL
);
