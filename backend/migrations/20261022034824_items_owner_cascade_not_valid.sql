-- Hand-written in place of the generated change (ADR go-backend D2): items.owner_id now cascades from users. The new
-- constraint is added NOT VALID, so this statement does not scan items while it holds the lock; the next migration
-- validates it in its own transaction. The end state equals schema.sql (make drift).
ALTER TABLE "items" DROP CONSTRAINT "items_owner_id_fkey", ADD CONSTRAINT "items_owner_id_fkey" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
