package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/PaoDevelopers/go-scfgs"
)

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
	Session struct {
		// Hex-encoded HMAC key, at least 32 bytes, for the signed
		// session cookies. There is no default and none is
		// generated: a key invented at startup would sign every
		// user out on every restart. Generate one with
		// `openssl rand -hex 32` and keep it out of version
		// control. Rotating it invalidates every live session,
		// which is the only revocation lever stateless sessions
		// have.
		Key string `scfgs:"key"`
	} `scfgs:"session"`
	OIDC struct {
		Client    string `scfgs:"client"`
		Authorize string `scfgs:"authorize"`
		JWKS      string `scfgs:"jwks"`
	} `scfgs:"oidc"`
	Admins map[string]struct{} `scfgs:"admins"`
}

// A configuration mistake must stop the process, not be absorbed.
//
// Everything below is a startup check, and startup is the only place
// these can be caught: a misspelt directive is indistinguishable from
// an absent one once the file is parsed, and an absent one is a zero,
// and a zero is a working value for most of these — a connect timeout
// of zero is "wait forever", and a max_conns of zero is not rejected
// by pgxpool.ParseConfig but silently replaced with its own default of
// max(4, NumCPU), which is not what anybody who typed a number meant.
// The failure is then a server that runs and behaves wrongly under
// load, months later, with nothing in the log tying it back to the
// line that was typed wrong.
var (
	ErrUnknownDirective = errors.New("unknown directive")
	ErrMissing          = errors.New("required setting is missing")
	ErrOutOfRange       = errors.New("setting is out of range")
)

// A comment in this format starts with '#'. '//' is not a comment: it
// parses as a directive named "//", which is why the unknown-directive
// check below reports the line rather than ignoring it.
const commentHint = "comments start with '#'; '//' is not a comment"

func Load(path string) (Config, error) {
	f, err := os.Open(path) //#nosec G304
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	var config Config

	decoder := scfgs.NewDecoder(bufio.NewReader(f))

	err = decoder.Decode(&config)
	if err != nil {
		return config, fmt.Errorf("decode config: %w", err)
	}

	// The first one is enough: an operator fixes them one at a time,
	// and the second is usually the same mistake as the first.
	if unknown := decoder.UnknownDirectives(); len(unknown) > 0 {
		name := unknown[0].Name
		if name == "//" {
			return config, fmt.Errorf("%w %q: %s",
				ErrUnknownDirective, name, commentHint)
		}

		return config, fmt.Errorf("%w %q", ErrUnknownDirective, name)
	}

	if err := config.validate(); err != nil {
		return config, err
	}

	return config, nil
}

// validate rejects a file that parsed but does not describe a server
// that can run.
//
// The floors are not opinions about tuning. Each one is a value below
// which the setting stops meaning what its name says: a zero duration
// is "no limit" to pgx, and a pool that may shrink to nothing still
// serves — slowly, by reconnecting for every request, which reads as
// the database being slow rather than as a line in this file.
func (c Config) validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"database.url", c.Database.URL},
		{"listen.address", c.Listen.Address},
		{"session.key", c.Session.Key},
		{"oidc.client", c.OIDC.Client},
		{"oidc.authorize", c.OIDC.Authorize},
		{"oidc.jwks", c.OIDC.JWKS},
	}
	for _, field := range required {
		if field.value == "" {
			return fmt.Errorf("%w: %s", ErrMissing, field.name)
		}
	}

	if len(c.Admins) == 0 {
		return fmt.Errorf("%w: admins (nobody could administer the system)",
			ErrMissing)
	}

	if c.Database.MaxConns < 1 {
		return fmt.Errorf("%w: database.max_conns must be at least 1",
			ErrOutOfRange)
	}

	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("%w: database.min_conns must be between 0 and max_conns",
			ErrOutOfRange)
	}

	// Durations are written in nanoseconds, which makes a wrong power
	// of ten easy and invisible: 5000000 looks like the 5000000000
	// beside it and is five milliseconds. A floor of one second on
	// each of these catches that, and no real deployment wants less.
	durations := []struct {
		name  string
		value time.Duration
	}{
		{"database.max_conn_lifetime", c.Database.MaxConnLifetime},
		{"database.max_conn_idle_time", c.Database.MaxConnIdleTime},
		{"database.health_check_period", c.Database.HealthCheckPeriod},
		{"database.connect_timeout", c.Database.ConnectTimeout},
	}
	for _, field := range durations {
		if field.value < time.Second {
			return fmt.Errorf(
				"%w: %s is %s; it is written in nanoseconds and must be at least 1s",
				ErrOutOfRange, field.name, field.value)
		}
	}

	return nil
}
