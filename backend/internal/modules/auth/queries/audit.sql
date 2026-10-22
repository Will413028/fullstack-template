-- InsertAuditEvent records one administrative change; callers run it in the change's own transaction.
-- name: InsertAuditEvent :exec
INSERT INTO audit_events (at, actor_id, action, target_id, request_id)
VALUES (@at, sqlc.narg('actor_id'), @action, @target_id, sqlc.narg('request_id'));
