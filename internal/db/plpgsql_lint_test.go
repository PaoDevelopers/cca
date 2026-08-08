package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// plpgsql_check over every plpgsql function in the schema.
//
// plpgsql bodies are the unchecked rung:
// PostgreSQL validates them only at execution, so a function
// referencing a dropped column loads silently. This recovers
// static checking as a standing test, with every warning class
// fatal (security, performance, and extra warnings included);
// if a concrete false positive ever appears, relax it here,
// per function, with a comment saying why.
func TestPlpgsqlLint(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	ctx := context.Background()

	exec(t, pool, `CREATE EXTENSION plpgsql_check`)

	rows, err := pool.Query(ctx, `
		SELECT f.proname, c.level, c.message, c.lineno, c.query
		FROM pg_proc f
		JOIN pg_language l ON l.oid = f.prolang
		JOIN pg_namespace n ON n.oid = f.pronamespace
		CROSS JOIN LATERAL plpgsql_check_function_tb(f.oid,
			security_warnings := true,
			performance_warnings := true,
			extra_warnings := true) c
		WHERE l.lanname = 'plpgsql' AND n.nspname = 'public'
		ORDER BY f.proname, c.lineno`)
	if err != nil {
		t.Fatalf("plpgsql_check: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			fn, level, message string
			lineno             pgtype.Int4
			query              pgtype.Text
		)

		if err := rows.Scan(&fn, &level, &message, &lineno, &query); err != nil {
			t.Fatalf("scan: %v", err)
		}

		t.Errorf("%s: %s %s (line %d: %s)",
			fn, level, message, lineno.Int32, query.String)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
