package db

// ExpectedSchemaVersion is the schema this build expects to find in the
// database. There are no automatic migrations: the application refuses
// to start against anything else, so an administrator who has not
// applied a schema change finds out immediately rather than through
// the first query that needs it.
//
// It has to agree with the value inserted by schemas/0001_version.sql.
// Nothing can enforce that across the two languages, so
// TestSchemaVersionMatchesTheSchema does.
const ExpectedSchemaVersion = 1

// SchemaFingerprint is the SHA-256 of every file under schemas/, in
// name order, each preceded by its name.
//
// The version check above is only as good as the discipline of bumping
// it, and that discipline had failed silently: the schema changed
// thirteen times while the version stayed at 1. Nothing said so,
// because nothing could — a version number cannot notice that the
// thing it versions has moved.
//
// This can. Editing any schema file changes the fingerprint, and
// TestSchemaFingerprintIsCurrent then fails until someone writes the
// new one down. Writing it down is the moment to ask whether the
// version needs bumping and whether docs/schema-changelog.md needs a
// line — which is the whole point, since the answer is usually yes and
// there was previously no moment at which anyone was asked.
//
// A fingerprint change is not automatically a version bump. Rewriting
// a comment, or a function whose behaviour is unchanged, moves the
// fingerprint and not the version. The changelog records which it was.
const SchemaFingerprint = "4fd75084bc9f5df6d74a31908a01807081bb0b400ae375d372d953278194d019"
