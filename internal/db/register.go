package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// RegisterTypes loads the schema's custom types into a connection's
// type map, so pgx can decode them — notably enum arrays like
// legal_sex[], which otherwise fail to scan with an unknown OID.
// Every pool connecting to a cca database must call this from its
// AfterConnect hook.
func RegisterTypes(ctx context.Context, conn *pgx.Conn) error {
	for _, name := range []string{"legal_sex", "legal_sex[]"} {
		t, err := conn.LoadType(ctx, name)
		if err != nil {
			return fmt.Errorf("load type %s: %w", name, err)
		}

		conn.TypeMap().RegisterType(t)
	}

	return nil
}
