CREATE TABLE "events" ("id" bigint NOT NULL GENERATED ALWAYS AS IDENTITY, "at" timestamptz NOT NULL, PRIMARY KEY ("id"));
COMMENT ON TABLE "events" IS 'append-only: retain 30 days by at';
CREATE TABLE "notes" ("id" bigint NOT NULL GENERATED ALWAYS AS IDENTITY, "body" text NOT NULL, PRIMARY KEY ("id"));
COMMENT ON TABLE "notes" IS 'append-only';
