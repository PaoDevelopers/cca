package db_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The version lives in two languages and cannot be shared between
// them, so bumping one and forgetting the other has to be caught here.
// Needs no database.
func TestSchemaVersionMatchesTheSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join("schemas", "0001_version.sql")

	content, err := os.ReadFile(path) //#nosec G304 -- fixed test data
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	match := regexp.MustCompile(
		`(?i)INSERT\s+INTO\s+schema_version\s*\(\s*version\s*\)\s*VALUES\s*\(\s*(\d+)\s*\)`,
	).FindSubmatch(content)
	if match == nil {
		t.Fatalf("no version insert found in %s; ExpectedSchemaVersion can no longer be checked against it", path)
	}

	inSchema, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatalf("parse version from %s: %v", path, err)
	}

	if inSchema != db.ExpectedSchemaVersion {
		t.Errorf("%s inserts version %d, but db.ExpectedSchemaVersion is %d", path, inSchema, db.ExpectedSchemaVersion)
	}
}

// The schema moved thirteen times while its version stayed at 1, and
// nothing noticed, because a version number cannot notice that the
// thing it versions has changed. This can.
//
// When it fails, the schema has been edited. Decide two things and
// then write the new fingerprint down:
//
//   - Does the version need bumping? Yes if a database created from
//     the old schema would no longer be one this build can serve.
//     No for a comment, or a rewrite that changes no behaviour.
//   - Does docs/schema-changelog.md need a line? Yes either way: it
//     is what an administrator reads to find out what to run.
//
// Writing the fingerprint down is not a chore to route around. It is
// the only moment at which anyone is asked those questions.
func TestSchemaFingerprintIsCurrent(t *testing.T) {
	t.Parallel()

	got := fingerprintSchemas(t)
	if got != db.SchemaFingerprint {
		t.Errorf("the schema has changed.\n"+
			"  fingerprint is %s\n"+
			"  recorded       %s\n"+
			"Update db.SchemaFingerprint, and decide whether "+
			"ExpectedSchemaVersion and docs/schema-changelog.md need "+
			"changing too.", got, db.SchemaFingerprint)
	}
}

// fingerprintSchemas hashes every schema file in name order, each
// preceded by its name, so that renaming or reordering a file counts
// as a change as much as editing one does.
func fingerprintSchemas(t *testing.T) string {
	t.Helper()

	names, err := filepath.Glob(filepath.Join("schemas", "*.sql"))
	if err != nil {
		t.Fatalf("list the schema files: %v", err)
	}

	if len(names) == 0 {
		t.Fatal("no schema files found; the fingerprint would be vacuous")
	}

	slices.Sort(names)

	sum := sha256.New()

	for _, name := range names {
		content, err := os.ReadFile(name) //#nosec G304 -- fixed test data
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		_, _ = sum.Write([]byte(filepath.Base(name)))
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write(content)
	}

	return hex.EncodeToString(sum.Sum(nil))
}
