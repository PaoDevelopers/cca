package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PaoDevelopers/go-scfgs"
)

// Config contains all supported CCA server settings.
type Config struct {
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
	TestAuth struct {
		Enabled     bool   `scfgs:"enabled"`
		AllowRemote bool   `scfgs:"allow_remote"`
		AccessKey   string `scfgs:"access_key"`
	} `scfgs:"test_auth"`
	DP42IK struct {
		ServiceID string `scfgs:"service_id"`
		KeyID     int    `scfgs:"key_id"`
		KeyB64    string `scfgs:"key_b64"`
	} `scfgs:"dp42ik"`
	Admins map[string]struct{} `scfgs:"admins"`
	// SSEBuf int                 `scfgs:"sse_buf"` // Not needed anymore
}

// Load reads and validates a configuration file at path.
func Load(path string) (Config, error) {
	f, err := os.Open(path) //#nosec G304
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer func() { _ = f.Close() }()

	var config Config
	err = scfgs.NewDecoder(bufio.NewReader(f)).Decode(&config)
	if err != nil {
		return config, fmt.Errorf("decode config: %w", err)
	}

	if err := validateConfig(config); err != nil {
		return config, err
	}

	return config, nil
}

func validateConfig(config Config) error {
	if config.TestAuth.Enabled && config.TestAuth.AllowRemote && len(strings.TrimSpace(config.TestAuth.AccessKey)) < 16 {
		return fmt.Errorf("test_auth.access_key must contain at least 16 characters when remote test authentication is allowed")
	}
	return nil
}
