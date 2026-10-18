-- name: CreateUser :one
INSERT INTO users (account, password_hash, role)
VALUES (@account, @password_hash, @role)
RETURNING id, account, role, is_disabled, created_at;

-- CreateUserIfAbsent leaves an existing account untouched and returns no row for it.
-- name: CreateUserIfAbsent :one
INSERT INTO users (account, password_hash, role)
VALUES (@account, @password_hash, @role)
ON CONFLICT (account) DO NOTHING
RETURNING id, account, role, is_disabled, created_at;

-- name: GetUser :one
SELECT id, account, role, is_disabled, created_at
FROM users
WHERE id = @id;

-- ListUsers returns all users, disabled ones included, newest first, after the cursor id when one is given. Only
-- admins call it (internal/httpapi requireRole).
-- name: ListUsers :many
SELECT id, account, role, is_disabled, created_at
FROM users
WHERE sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid
ORDER BY id DESC
LIMIT @row_limit;

-- name: GetLoginUser :one
SELECT id, account, password_hash, role, is_disabled, created_at
FROM users
WHERE account = @account;

-- name: SetUserDisabled :one
UPDATE users
SET is_disabled = @is_disabled, updated_at = now()
WHERE id = @id
RETURNING id, account, role, is_disabled, created_at;
