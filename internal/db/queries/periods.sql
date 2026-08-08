-- name: GetPeriods :many
SELECT id, name, sort_order FROM periods ORDER BY sort_order, id;

-- name: NewPeriod :exec
INSERT INTO periods (id, name, sort_order)
SELECT sqlc.arg(id), sqlc.arg(name), COALESCE(MAX(sort_order), 0) + 1
FROM periods;

-- name: DeletePeriod :execrows
DELETE FROM periods WHERE id = $1;

-- name: RenamePeriod :execrows
UPDATE periods SET name = $2 WHERE id = $1;

-- name: SetPeriodOrder :exec
SELECT set_period_order($1);
