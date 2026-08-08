package web //nolint:testpackage

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The error vocabulary lives in two languages: the schema raises
// SQLSTATEs, and this package turns them into responses. Nothing can
// make them agree, so this does.
//
// An unclassified SQLSTATE is not a cosmetic gap. It becomes a 500
// with the database's own message in it, logged at error level — so an
// ordinary, expected refusal (a student clicking Drop on a placement
// an administrator fixed) reads as a server fault, pages whoever
// watches the logs, and shows the user a generic failure instead of
// the reason. That is how YKG03 was missed.
func TestEverySQLStateTheSchemaRaisesIsClassified(t *testing.T) {
	t.Parallel()

	raised := sqlStatesRaisedBySchema(t)
	if len(raised) == 0 {
		t.Fatal("found no RAISE ... USING ERRCODE in the schema; this test is not testing anything")
	}

	for _, state := range raised {
		// A minimal error of that state. The payload-bearing ones are
		// given a decodable payload, because their classification
		// depends on it.
		//exhaustruct:ignore
		err := &pgconn.PgError{Code: state, Message: "raised by the schema"}

		switch state {
		case "YKV01":
			err.Detail = `[{"student_id":"s1","rule":"clash","code":"c",` +
				`"other_course_id":null,"period_id":null,"detail":"d"}]`
		case "YKD01":
			err.Detail = `[{"index":1,"id":"x","sqlstate":"23514",` +
				`"constraint":"","message":"m"}]`
		}

		status, detail, ok := dbErrorDetail(err, false)
		if !ok {
			t.Errorf("SQLSTATE %s is raised by the schema but not classified; "+
				"it would reach the user as a 500", state)

			continue
		}

		if status >= 500 {
			t.Errorf("SQLSTATE %s classifies as %d; a rejection the schema "+
				"raises deliberately is not a server fault", state, status)
		}

		if detail.Code == "" {
			t.Errorf("SQLSTATE %s classifies with an empty code", state)
		}
	}
}

// sqlStatesRaisedBySchema reads every RAISE ... USING ERRCODE = '...'
// out of the schema files. Reading the SQL rather than listing the
// states here is the point: a state added to the schema tomorrow is
// caught without anyone remembering to update this.
func sqlStatesRaisedBySchema(t *testing.T) []string {
	t.Helper()

	dir := filepath.Join("..", "db", "schemas")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	pattern := regexp.MustCompile(`ERRCODE\s*=\s*'([0-9A-Za-z]{5})'`)

	var states []string

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name())) //#nosec G304 -- fixed test data
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}

		for _, match := range pattern.FindAllSubmatch(content, -1) {
			state := string(match[1])
			if !slices.Contains(states, state) {
				states = append(states, state)
			}
		}
	}

	slices.Sort(states)

	return states
}
