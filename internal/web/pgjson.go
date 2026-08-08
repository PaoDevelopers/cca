package web

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Conversions between pgx's nullable scalars and the pointers the JSON
// layer wants. A pgtype value marshals as an object with its own
// Valid/Int64 fields, which is an artefact of the driver rather than
// anything the API should expose; null is the honest wire form of
// absence, and a pointer is how encoding/json spells it.
//
// These go in both directions because absence is meaningful on the way
// in too: a request omitting a cap is asking for no cap, which is not
// the same as asking for zero.

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}

	t := v.Time

	return &t
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}

	s := v.String

	return &s
}

func int64Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}

	n := v.Int64

	return &n
}

func pgTime(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		//exhaustruct:ignore
		return pgtype.Timestamptz{Valid: false}
	}

	//exhaustruct:ignore
	return pgtype.Timestamptz{Time: *v, Valid: true}
}

func pgInt64(v *int64) pgtype.Int8 {
	if v == nil {
		//exhaustruct:ignore
		return pgtype.Int8{Valid: false}
	}

	//exhaustruct:ignore
	return pgtype.Int8{Int64: *v, Valid: true}
}
