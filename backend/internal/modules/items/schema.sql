-- items module: desired state of its table. Atlas generates migrations from this file (make migration); never edit
-- applied migrations. Type rules follow docs/adr/2026-10-18-go-backend.md (UUIDv7 keys, text with CHECK, timestamptz).

-- owner_id -> users.id is the only cross-module foreign key (plan I4); the items queries never read users. No
-- cascade: deleting a user who still owns items fails instead of silently removing them.
CREATE TABLE items (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  title       text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  description text,
  owner_id    uuid        NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Keyset pagination: WHERE owner_id = $1 AND id < $cursor ORDER BY id DESC.
CREATE INDEX items_owner_id_id_idx ON items (owner_id, id DESC);
