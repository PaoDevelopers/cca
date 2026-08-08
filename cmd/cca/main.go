package main

import (
	"flag"
	"io"
	"log/slog"
	"os"

	"github.com/PaoDevelopers/cca/internal/web"
)

func main() {
	//exhaustruct:ignore
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))

	var configPath string

	var mintSession string

	flag.StringVar(&configPath, "c", "cca.conf", "path to configuration file")
	// Mints a session cookie value and exits, without serving or
	// touching the database.
	//
	// It grants nothing its holder did not already have: it needs the
	// configuration file, and anyone who can read that can read the
	// signing key and mint sessions with thirty lines of any language.
	// What it buys is that the end-to-end tests, and anyone debugging
	// an account, use the real implementation of the cookie format
	// rather than a second copy of it that can drift.
	flag.StringVar(&mintSession, "session", "",
		"mint a session cookie for <role>:<subject> (student:s22537) and exit")
	flag.Parse()

	if mintSession != "" {
		value, err := web.MintSession(configPath, mintSession)
		if err != nil {
			slog.Error("cannot mint session", slog.Any("error", err))
			os.Exit(1)
		}

		// The cookie value alone on stdout, so a caller can capture it
		// without parsing anything. The logs go elsewhere.
		if _, err := io.WriteString(os.Stdout, value+"\n"); err != nil {
			slog.Error("cannot write session", slog.Any("error", err))
			os.Exit(1)
		}

		return
	}

	web.Run(configPath)
}
