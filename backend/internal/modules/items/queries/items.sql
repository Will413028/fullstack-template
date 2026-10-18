-- Every query is scoped to the owner (plan I6): another user's item behaves as if it did not exist. Updates and
-- deletes report the rows they touched, so "not yours" is a 404 rather than a silent success.

-- name: CreateItem :one
INSERT INTO items (title, description, owner_id)
VALUES (@title, @description, @owner_id)
RETURNING id, title, description, owner_id, created_at, updated_at;

-- name: GetItem :one
SELECT id, title, description, owner_id, created_at, updated_at
FROM items
WHERE id = @id AND owner_id = @owner_id;

-- ListItems returns the owner's items newest first (UUIDv7 ids sort by creation time), after the cursor id when
-- one is given.
-- name: ListItems :many
SELECT id, title, description, owner_id, created_at, updated_at
FROM items
WHERE owner_id = @owner_id AND (sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- UpdateItem applies a merge patch: a null title leaves it unchanged (the contract forbids title: null), and
-- set_description says whether description was sent, so null can clear it.
-- name: UpdateItem :one
UPDATE items
SET title       = coalesce(sqlc.narg('title'), title),
    description = CASE WHEN @set_description::boolean THEN sqlc.narg('description') ELSE description END,
    updated_at  = now()
WHERE id = @id AND owner_id = @owner_id
RETURNING id, title, description, owner_id, created_at, updated_at;

-- name: DeleteItem :execrows
DELETE FROM items
WHERE id = @id AND owner_id = @owner_id;
