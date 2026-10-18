-- Create "login_throttles" table
CREATE TABLE "login_throttles" (
  "account_key" text NOT NULL,
  "failures" bigint NOT NULL DEFAULT 0,
  "next_allowed_at" timestamptz NULL,
  "updated_at" timestamptz NOT NULL,
  PRIMARY KEY ("account_key"),
  CONSTRAINT "login_throttles_failures_check" CHECK (failures >= 0)
);
-- Create "users" table
CREATE TABLE "users" (
  "id" uuid NOT NULL DEFAULT uuidv7(),
  "account" text NOT NULL,
  "password_hash" text NOT NULL,
  "role" text NOT NULL DEFAULT 'user',
  "is_disabled" boolean NOT NULL DEFAULT false,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "users_account_key" UNIQUE ("account"),
  CONSTRAINT "users_account_check" CHECK ((account = lower(account)) AND ((char_length(account) >= 3) AND (char_length(account) <= 50))),
  CONSTRAINT "users_role_check" CHECK (role = ANY (ARRAY['admin'::text, 'user'::text]))
);
-- Create "items" table
CREATE TABLE "items" (
  "id" uuid NOT NULL DEFAULT uuidv7(),
  "title" text NOT NULL,
  "description" text NULL,
  "owner_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "items_owner_id_fkey" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "items_title_check" CHECK ((char_length(title) >= 1) AND (char_length(title) <= 200))
);
-- Create index "items_owner_id_id_idx" to table: "items"
CREATE INDEX "items_owner_id_id_idx" ON "items" ("owner_id", "id" DESC);
-- Create "sessions" table
CREATE TABLE "sessions" (
  "token_hash" bytea NOT NULL,
  "id" uuid NOT NULL DEFAULT uuidv7(),
  "user_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL,
  "last_used_at" timestamptz NOT NULL,
  "expires_at" timestamptz NOT NULL,
  PRIMARY KEY ("token_hash"),
  CONSTRAINT "sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "sessions_token_hash_check" CHECK (octet_length(token_hash) = 32)
);
-- Create index "sessions_id_key" to table: "sessions"
CREATE UNIQUE INDEX "sessions_id_key" ON "sessions" ("id");
-- Create index "sessions_user_id_id_idx" to table: "sessions"
CREATE INDEX "sessions_user_id_id_idx" ON "sessions" ("user_id", "id" DESC);
