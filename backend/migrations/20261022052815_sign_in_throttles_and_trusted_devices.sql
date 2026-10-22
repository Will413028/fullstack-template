-- Create "sign_in_throttles" table
CREATE TABLE "sign_in_throttles" (
  "account_key" text NOT NULL,
  "client_key" text NOT NULL,
  "failures" bigint NOT NULL DEFAULT 0,
  "next_allowed_at" timestamptz NULL,
  "updated_at" timestamptz NOT NULL,
  PRIMARY KEY ("account_key", "client_key"),
  CONSTRAINT "sign_in_throttles_failures_check" CHECK (failures >= 0)
);
-- Create "trusted_devices" table
CREATE TABLE "trusted_devices" (
  "token_hash" bytea NOT NULL,
  "id" uuid NOT NULL DEFAULT uuidv7(),
  "user_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL,
  "expires_at" timestamptz NOT NULL,
  PRIMARY KEY ("token_hash"),
  CONSTRAINT "trusted_devices_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "trusted_devices_token_hash_check" CHECK (octet_length(token_hash) = 32)
);
-- Create index "trusted_devices_id_key" to table: "trusted_devices"
CREATE UNIQUE INDEX "trusted_devices_id_key" ON "trusted_devices" ("id");
-- Create index "trusted_devices_user_id_idx" to table: "trusted_devices"
CREATE INDEX "trusted_devices_user_id_idx" ON "trusted_devices" ("user_id");
