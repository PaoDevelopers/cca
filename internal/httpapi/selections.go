package httpapi

import (
	"git.sr.ht/~runxiyu/cca/internal/selections"
	db "git.sr.ht/~runxiyu/cca/internal/store/sqlc"
)

func cartesianSelectionBatch(
	studentIDs []int64,
	courseIDs []string,
	periodIDs []string,
	selectionType db.SelectionType,
) db.NewSelectionsBatchParams {
	return selections.CartesianBatch(studentIDs, courseIDs, periodIDs, selectionType)
}
