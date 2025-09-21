package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/PaoDevelopers/go-scfgs"
)

type Config struct {
	URL      string `scfgs:"url"`
	Database string `scfgs:"database"`
	Listen   struct {
		Protocol  string `scfgs:"protocol"`
		Network   string `scfgs:"network"`
		Address   string `scfgs:"address"`
		Transport string `scfgs:"transport"`
		TLS       struct {
			Cert string `scfgs:"cert"`
			Key  string `scfgs:"key"`
		} `scfgs:"tls"`
	} `scfgs:"listen"`
	OIDC struct {
		Client    string `scfgs:"client"`
		Authorize string `scfgs:"authorize"`
		JWKS      string `scfgs:"jwks"`
	} `scfgs:"oidc"`
	Admins map[string]struct{} `scfgs:"admins"`
}

func loadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}

	var config Config
	err = scfgs.NewDecoder(bufio.NewReader(f)).Decode(&config)
	if err != nil {
		return config, fmt.Errorf("decode config: %w", err)
	}

	return config, nil
}
