-- Create "audit_events" table
CREATE TABLE "audit_events" (
  "id" uuid NOT NULL DEFAULT uuidv7(),
  "at" timestamptz NOT NULL,
  "actor_id" uuid NULL,
  "action" text NOT NULL,
  "target_id" uuid NOT NULL,
  "request_id" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "audit_events_action_check" CHECK (action = ANY (ARRAY['user.disabled'::text, 'user.enabled'::text, 'user.deleted'::text, 'user.unlocked'::text, 'admin.recovered'::text]))
);
-- Create index "audit_events_at_idx" to table: "audit_events"
CREATE INDEX "audit_events_at_idx" ON "audit_events" ("at");
-- Set comment to table: "audit_events"
COMMENT ON TABLE "audit_events" IS 'append-only: retain 365 days by at';
