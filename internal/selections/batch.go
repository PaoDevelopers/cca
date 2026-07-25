package selections

import "git.sr.ht/~runxiyu/cca/internal/store/sqlc"

// CartesianBatch expands students and aligned course-period pairs into SQL arrays.
func CartesianBatch(
	studentIDs []int64,
	courseIDs []string,
	periodIDs []string,
	selectionType db.SelectionType,
) db.NewSelectionsBatchParams {
	batchSize := len(studentIDs) * len(courseIDs)
	params := db.NewSelectionsBatchParams{
		StudentIds:     make([]int64, 0, batchSize),
		CourseIds:      make([]string, 0, batchSize),
		PeriodIds:      make([]string, 0, batchSize),
		SelectionTypes: make([]db.SelectionType, 0, batchSize),
	}
	for _, studentID := range studentIDs {
		for index, courseID := range courseIDs {
			params.StudentIds = append(params.StudentIds, studentID)
			params.CourseIds = append(params.CourseIds, courseID)
			params.PeriodIds = append(params.PeriodIds, periodIDs[index])
			params.SelectionTypes = append(params.SelectionTypes, selectionType)
		}
	}
	return params
}
