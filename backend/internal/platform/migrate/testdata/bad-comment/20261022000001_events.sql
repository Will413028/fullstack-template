CREATE TABLE "events" ("id" bigint NOT NULL GENERATED ALWAYS AS IDENTITY, "at" timestamptz NOT NULL, PRIMARY KEY ("id"));
COMMENT ON TABLE "events" IS 'append-only: keep for a year';
