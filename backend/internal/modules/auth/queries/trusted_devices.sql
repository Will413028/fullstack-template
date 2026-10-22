-- name: InsertTrustedDevice :exec
INSERT INTO trusted_devices (token_hash, user_id, created_at, expires_at)
VALUES (@token_hash, @user_id, @now, @expires_at);

-- GetTrustedDevice returns the unexpired device of the token with its user's account name, so the caller can tell
-- whether it belongs to the account signing in.
-- name: GetTrustedDevice :one
SELECT d.id, u.account
FROM trusted_devices d
JOIN users u ON u.id = d.user_id
WHERE d.token_hash = @token_hash AND d.expires_at > @now;

-- name: DeleteUserTrustedDevices :exec
DELETE FROM trusted_devices
WHERE user_id = @user_id;

-- name: DeleteExpiredTrustedDevices :execrows
DELETE FROM trusted_devices
WHERE expires_at <= @now;
