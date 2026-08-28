package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The harness applies internal/db/schemas/ (or $CCA_SCHEMA_DIR, which
// is how mutation runs point at a mutated copy) to a template database
// once per test run; each test clones the template, so tests are
// parallel, may commit freely, and leave nothing behind.
// Connection settings come from the usual PG* environment; a missing
// or unreachable server fails the tests rather than skipping them.

// The template is a per-run staging area, nothing more:
// built once per test-binary execution (per package, per invocation),
// cloned cheaply by each test, dropped after m.Run.
// Its name is per-process so concurrent executions — two go test
// commands, or several package binaries inside one `go test ./...` —
// cannot interact by construction;
// nothing is shared, so no cache coherence exists to get wrong.
// A killed run leaks its template
// (teardown never fires; the prefix identifies strays for manual
// DROP, and the next run of the same PID reclaims it);
// that wart is accepted rather than engineered away,
// since any startup sweep could reap a concurrent run's live template.
func templateName() string {
	return fmt.Sprintf("cca_test_tpl_%d", os.Getpid())
}

// TestMain builds the template database once for this run;
// individual tests clone it.
func TestMain(m *testing.M) {
	if err := buildTemplate(); err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		os.Exit(1)
	}

	code := m.Run()

	dropTemplate()
	os.Exit(code)
}

func dropTemplate() {
	ctx := context.Background()

	admin, err := connectTo(ctx, "", false)
	if err != nil {
		return
	}
	defer admin.Close()

	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+templateName())
}

func schemaDir() string {
	if dir := os.Getenv("CCA_SCHEMA_DIR"); dir != "" {
		return dir
	}

	return "schemas"
}

// connectTo returns a pool on the named database,
// with "" meaning the maintenance database
// (for CREATE/DROP DATABASE).
// register loads the schema's custom types into every connection,
// so it must be false for databases that do not hold the schema yet
// (the maintenance database, a template mid-build).
func connectTo(ctx context.Context, database string, register bool) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.ConnConfig.Database = database
	if database == "" {
		cfg.ConnConfig.Database = "postgres"
	}

	if register {
		cfg.AfterConnect = db.RegisterTypes
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", cfg.ConnConfig.Database, err)
	}

	return pool, nil
}

func adminConn(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := connectTo(t.Context(), "", false)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func buildTemplate() error {
	ctx := context.Background()

	admin, err := connectTo(ctx, "", false)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer admin.Close()

	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+templateName())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+templateName()); err != nil {
		return fmt.Errorf("create template: %w", err)
	}

	tpl, err := connectTo(ctx, templateName(), false)
	if err != nil {
		return fmt.Errorf("connect template: %w", err)
	}
	defer tpl.Close()

	dir := os.DirFS(schemaDir())

	files, err := fs.Glob(dir, "*.sql")
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no schema files in %q: %w", schemaDir(), err)
	}

	slices.Sort(files)

	for _, f := range files {
		sql, err := fs.ReadFile(dir, f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		if _, err := tpl.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", f, err)
		}
	}

	return nil
}

// fresh clones the template into a database owned by this test and
// returns a pool and Queries on it.
func fresh(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()

	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}

	name := "cca_test_" + hex.EncodeToString(suffix[:])

	ctx := context.Background()

	admin := adminConn(t)
	if _, err := admin.Exec(ctx,
		fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateName())); err != nil {
		t.Fatalf("clone template: %v", err)
	}

	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name)
	})

	pool, err := connectTo(ctx, name, true)
	if err != nil {
		t.Fatalf("connect %s: %v", name, err)
	}

	t.Cleanup(pool.Close)

	return pool, db.New(pool)
}

// pgError demands err be a PostgreSQL error with exactly this
// SQLSTATE and returns it for payload inspection.
func pgError(t *testing.T, err error, state string) *pgconn.PgError {
	t.Helper()

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("want SQLSTATE %s, got %v", state, err)
	}

	if pgErr.Code != state {
		t.Fatalf("want SQLSTATE %s, got %s: %s (detail %q)",
			state, pgErr.Code, pgErr.Message, pgErr.Detail)
	}

	return pgErr
}

// expectState demands err be a PostgreSQL error with exactly this
// SQLSTATE.
func expectState(t *testing.T, err error, state string) {
	t.Helper()
	_ = pgError(t, err, state)
}

// violation is one element of a YKV01 or YKD01 DETAIL payload.
type violation struct {
	StudentID     *string `json:"student_id"`
	Rule          string  `json:"rule"`
	Code          string  `json:"code"`
	OtherCourseID *string `json:"other_course_id"`
	PeriodID      *string `json:"period_id"`
	Detail        string  `json:"detail"`
}

// expectCodes demands err be YKV01 carrying exactly these codes,
// order-insensitively, and returns the decoded violations.
func expectCodes(t *testing.T, err error, codes ...string) []violation {
	t.Helper()

	pgErr := pgError(t, err, "YKV01")

	var vs []violation
	if err := json.Unmarshal([]byte(pgErr.Detail), &vs); err != nil {
		t.Fatalf("decode DETAIL %q: %v", pgErr.Detail, err)
	}

	got := make([]string, 0, len(vs))
	for _, v := range vs {
		got = append(got, v.Code)
	}

	slices.Sort(got)

	want := slices.Clone(codes)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("violation codes = %v, want %v", got, want)
	}

	return vs
}

// exec runs raw scaffolding SQL (seeds, backdoor assertions) and fails
// the test on error.
func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()

	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// count returns a scalar bigint from scaffolding SQL.
func count(t *testing.T, pool *pgxpool.Pool, sql string) int64 {
	t.Helper()

	var n int64
	if err := pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}

	return n
}

// capacity spells a course's cap for the write functions, whose
// max_students is nullable: NULL is no cap at all, and is a different
// setting from 0, which is a cap that admits nobody. Tests say which
// they mean rather than leaving a zero value to decide.
func capacity(n int64) pgtype.Int8 {
	//exhaustruct:ignore
	return pgtype.Int8{Int64: n, Valid: true}
}

// uncapped is the course that takes everyone.
//
//nolint:gochecknoglobals
//exhaustruct:ignore
var uncapped = pgtype.Int8{Valid: false}
