-- name: InsertSession :one
INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at)
VALUES (@token_hash, @user_id, @now, @now, @expires_at)
RETURNING id, created_at, last_used_at, expires_at;

-- GetSession returns the session with its user; expiry and is_disabled are checked by the caller.
-- name: GetSession :one
SELECT s.id AS session_id, s.last_used_at, s.expires_at, u.id, u.account, u.role, u.is_disabled, u.created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash;

-- name: TouchSession :exec
UPDATE sessions
SET last_used_at = @now
WHERE token_hash = @token_hash;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = @token_hash;

-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE user_id = @user_id;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= @now OR last_used_at <= @idle_before;

-- The session resource (docs/adr/2026-10-18-api-and-auth-baseline.md D9): ListUserSessions, GetUserSession and
-- DeleteUserSession are scoped to the user (another user's is not found) and see only live sessions, within the
-- limits Authenticate applies (before expires_at, and used after idle_before), so an expired session that Cleanup has
-- not removed yet is not found either.

-- ListUserSessions returns the user's live sessions newest first (UUIDv7 ids sort by creation time), after the
-- cursor id when one is given.
-- name: ListUserSessions :many
SELECT id, created_at, last_used_at, expires_at
FROM sessions
WHERE user_id = @user_id AND expires_at > @now AND last_used_at > @idle_before
  AND (sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- name: GetUserSession :one
SELECT id, created_at, last_used_at, expires_at
FROM sessions
WHERE id = @id AND user_id = @user_id AND expires_at > @now AND last_used_at > @idle_before;

-- name: DeleteUserSession :execrows
DELETE FROM sessions
WHERE id = @id AND user_id = @user_id AND expires_at > @now AND last_used_at > @idle_before;

-- GetSessionCreatedAt is when the user's session was issued, that is when they last signed in with it.
-- name: GetSessionCreatedAt :one
SELECT created_at
FROM sessions
WHERE id = @id AND user_id = @user_id;
