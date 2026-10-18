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
