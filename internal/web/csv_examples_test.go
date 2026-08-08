package web //nolint:testpackage

import (
	"encoding/csv"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The example spreadsheets shipped alongside the import forms, checked
// against the importers that have to read them.
//
// They had drifted a whole rewrite behind: the course example still
// used "membership" and "group" for what are now "invite_only" and
// "category", the roster example gave grades as "Year 9" where an id
// belongs and student ids as bare numbers where a localpart belongs,
// and the third file was for an import that no longer exists. Every
// one of them was rejected outright by the code shipped in the same
// binary.
//
// Nothing linked to them, which is how they got that way. Now the
// import forms do, so the drift has a cost — and this has a test.

func exampleRows(t *testing.T, name string) [][]string {
	t.Helper()

	file, err := staticFS.Open(path.Join("static", name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}

	defer func() {
		_ = file.Close()
	}()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}

	if len(rows) < 2 {
		t.Fatalf("%s has %d rows; an example with no example in it is not one", name, len(rows))
	}

	return rows
}

func TestTheShippedExamplesHaveTheHeadersTheImportersWant(t *testing.T) {
	t.Parallel()

	for name, want := range map[string][]string{
		"students_example.csv":    studentImportColumns,
		"courses_example.csv":     courseImportColumns,
		"enrollments_example.csv": enrollmentImportColumns,
	} {
		rows := exampleRows(t, name)
		if got := rows[0]; !slices.Equal(got, want) {
			t.Errorf("%s header:\n got %v\nwant %v", name, got, want)
		}

		// Every row the same width as the header, or the CSV reader
		// refuses the file before any of it is read.
		for i, row := range rows {
			if len(row) != len(want) {
				t.Errorf("%s row %d has %d fields, want %d", name, i+1, len(row), len(want))
			}
		}
	}
}

// The values, not just the columns. An example that parses but names a
// grade "Year 9" teaches the administrator the wrong thing twice: once
// when their own file is rejected, and again when they cannot see why.
func TestTheShippedExamplesUseTheIdentifierFormsTheDomainsRequire(t *testing.T) {
	t.Parallel()

	// entity_id is uppercase; localpart is lowercase. Checked here
	// rather than against the domains themselves because these files
	// are read by a person before they are read by anything else.
	upper := func(t *testing.T, where string, value string) {
		t.Helper()

		if value != "" && value != strings.ToUpper(value) {
			t.Errorf("%s = %q; ids are uppercase", where, value)
		}
	}

	students := exampleRows(t, "students_example.csv")
	for _, row := range students[1:] {
		if id := row[0]; id != strings.ToLower(id) {
			t.Errorf("student id %q is not a lowercase email localpart", id)
		}

		upper(t, "student grade", row[2])
	}

	courses := exampleRows(t, "courses_example.csv")
	for _, row := range courses[1:] {
		upper(t, "course id", row[courseColID])
		upper(t, "course category", row[courseColCategory])

		for _, period := range splitList(row[courseColPeriods]) {
			upper(t, "course period", period)
		}

		for _, grade := range splitList(row[courseColGrades]) {
			upper(t, "course allowed grade", grade)
		}

		// The two booleans are spelt the way the importer parses them,
		// not "invite_only"/"self_selectable" as the old file had it.
		if flag := row[courseColInviteOnly]; flag != "true" && flag != "false" {
			t.Errorf("invite_only = %q, want true or false", flag)
		}
	}

	enrollments := exampleRows(t, "enrollments_example.csv")
	for _, row := range enrollments[1:] {
		upper(t, "enrollment course", row[0])

		if id := row[1]; id != strings.ToLower(id) {
			t.Errorf("enrollment student %q is not a lowercase email localpart", id)
		}

		for i, flag := range row[2:] {
			if flag != "true" && flag != "false" {
				t.Errorf("enrollment column %d = %q, want true or false", i+3, flag)
			}
		}
	}
}

// Nothing under static/ should be an example for an import that no
// longer exists. The third file was one of those for an entire
// rewrite, and it survived because nothing named it.
func TestEveryShippedExampleBelongsToAnImportThatExists(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(staticFS, "static")
	if err != nil {
		t.Fatalf("read static: %v", err)
	}

	known := map[string]bool{
		"students_example.csv":    true,
		"courses_example.csv":     true,
		"enrollments_example.csv": true,
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".csv") {
			continue
		}

		if !known[name] {
			t.Errorf("static/%s is shipped but belongs to no import", name)
		}
	}
}

// One JSON package across the tree.
//
// v1 and v2 are not the same library wearing two names: v2 rejects
// duplicate object members, treats unknown members as an option rather
// than a default, and formats differently. A file that imports v1
// while everything around it uses v2 will do something subtly other
// than what its neighbours do, and nothing about the call sites says
// which one is in scope — both are spelt `json.Unmarshal`.
//
// depguard is turned off in .golangci.yaml on purpose, so this is
// where the rule lives instead.
func TestNothingImportsTheOldJSONPackage(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("locate the repository root: %v", err)
	}

	fset := token.NewFileSet()

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			// The vendored frontend, and the stashed rewrite, are not
			// ours to hold to this.
			if entry.Name() == "node_modules" || entry.Name() == "rewrite" {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}

		for _, imported := range file.Imports {
			if imported.Path.Value == `"encoding/json"` {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s imports encoding/json; this tree uses encoding/json/v2", rel)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
