package selections

import (
	"reflect"
	"testing"

	"git.sr.ht/~runxiyu/cca/internal/store/sqlc"
)

func TestCartesianSelectionBatchProducesAlignedArrays(t *testing.T) {
	got := CartesianBatch(
		[]int64{2, 1},
		[]string{"B", "A"},
		[]string{"Tuesday CCA 1", "Monday CCA 1"},
		db.SelectionTypeInvite,
	)
	if want := []int64{2, 2, 1, 1}; !reflect.DeepEqual(got.StudentIds, want) {
		t.Fatalf("student IDs = %#v, want %#v", got.StudentIds, want)
	}
	if want := []string{"B", "A", "B", "A"}; !reflect.DeepEqual(got.CourseIds, want) {
		t.Fatalf("course IDs = %#v, want %#v", got.CourseIds, want)
	}
	if want := []string{"Tuesday CCA 1", "Monday CCA 1", "Tuesday CCA 1", "Monday CCA 1"}; !reflect.DeepEqual(got.PeriodIds, want) {
		t.Fatalf("period IDs = %#v, want %#v", got.PeriodIds, want)
	}
	if len(got.SelectionTypes) != 4 {
		t.Fatalf("selection types length = %d, want 4", len(got.SelectionTypes))
	}
	for i, selectionType := range got.SelectionTypes {
		if selectionType != string(db.SelectionTypeInvite) {
			t.Fatalf("selection type %d = %q, want %q", i, selectionType, db.SelectionTypeInvite)
		}
	}
}
