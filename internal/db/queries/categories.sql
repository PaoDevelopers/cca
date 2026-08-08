-- name: GetCategories :many
SELECT id, name FROM categories ORDER BY id;

-- name: NewCategory :exec
INSERT INTO categories (id, name) VALUES ($1, $2);

-- name: RenameCategory :execrows
UPDATE categories SET name = $2 WHERE id = $1;

-- name: DeleteCategory :execrows
DELETE FROM categories WHERE id = $1;
