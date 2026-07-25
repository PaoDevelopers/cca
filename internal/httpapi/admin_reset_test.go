package httpapi

import "testing"

func TestAdminResetConfirmation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scope string
		want  string
		ok    bool
	}{
		{scope: "selections", want: "RESET SELECTIONS", ok: true},
		{scope: "courses", want: "RESET COURSES", ok: true},
		{scope: "students", want: "RESET STUDENTS", ok: true},
		{scope: "all", ok: false},
		{scope: "", ok: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.scope, func(t *testing.T) {
			t.Parallel()
			got, ok := adminResetConfirmation(test.scope)
			if ok != test.ok || got != test.want {
				t.Fatalf("adminResetConfirmation(%q) = %q, %v; want %q, %v", test.scope, got, ok, test.want, test.ok)
			}
		})
	}
}
