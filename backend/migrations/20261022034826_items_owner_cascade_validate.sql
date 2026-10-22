-- Validates the constraint the previous migration added NOT VALID; this takes only a lock that allows reads and
-- writes while it scans (ADR go-backend D2).
ALTER TABLE "items" VALIDATE CONSTRAINT "items_owner_id_fkey";
