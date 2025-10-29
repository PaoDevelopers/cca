package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/PaoDevelopers/go-scfgs"
)

type Config struct {
	URL      string `scfgs:"url"`
	Database struct {
		URL               string        `scfgs:"url"`
		MaxConns          int32         `scfgs:"max_conns"`
		MinConns          int32         `scfgs:"min_conns"`
		MaxConnLifetime   time.Duration `scfgs:"max_conn_lifetime"`
		MaxConnIdleTime   time.Duration `scfgs:"max_conn_idle_time"`
		HealthCheckPeriod time.Duration `scfgs:"health_check_period"`
		ConnectTimeout    time.Duration `scfgs:"connect_timeout"`
	} `scfgs:"database"`
	Listen struct {
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
	SSEBuf int                 `scfgs:"sse_buf"`
}

func loadConfig(path string) (Config, error) {
	f, err := os.Open(path) //#nosec G304
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
