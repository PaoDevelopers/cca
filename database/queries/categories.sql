---- Categories

-- name: GetCategories :many
SELECT id
FROM categories
ORDER BY id;

-- name: NewCategory :exec
INSERT INTO categories (id)
VALUES ($1);

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = $1;
