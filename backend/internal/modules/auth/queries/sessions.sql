-- name: InsertSession :exec
INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at)
VALUES (@token_hash, @user_id, @now, @now, @expires_at);

-- GetSession returns the session with its user; expiry and is_disabled are checked by the caller.
-- name: GetSession :one
SELECT s.last_used_at, s.expires_at, u.id, u.account, u.role, u.is_disabled, u.created_at
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
