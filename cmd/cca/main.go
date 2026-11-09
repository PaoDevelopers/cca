package main

import (
	"flag"
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

	var devLogin bool

	flag.StringVar(&configPath, "c", "cca.conf", "path to configuration file")
	flag.BoolVar(&devLogin, "dev-login", false,
		"sign in as any account without the identity provider (loopback listeners only)")
	flag.Parse()

	web.Run(configPath, devLogin)
}
