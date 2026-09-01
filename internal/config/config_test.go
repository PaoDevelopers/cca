package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaoDevelopers/cca/internal/config"
)

// A working file, as a base for the tests that break one thing in it.
//
// scfgs already refuses a file with a directive missing, and a
// directive with no parameter at all, so the tests below give a
// directive an empty value instead: that is the one shape the decoder
// accepts and cannot judge.
const good = `database {
	url postgres:///cca
	max_conns 80
	min_conns 10
	max_conn_lifetime 14400000000000
	max_conn_idle_time 1200000000000
	health_check_period 60000000000
	connect_timeout 5000000000
}

listen {
	protocol http
	network tcp
	address :8080
	transport plain
	tls {
		cert ""
		key ""
	}
}

session {
	key 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
}

oidc {
	client c
	authorize https://example.invalid/authorize
	jwks https://example.invalid/keys
}

admins {
	someone
}
`

func write(t *testing.T, body string) string {
	t.Helper()

	// t.TempDir is the test framework's own directory, so the path is
	// not attacker-influenced however the analysis reads it.
	path := filepath.Join(t.TempDir(), "cca.conf")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil { //#nosec G703
		t.Fatalf("write the config: %v", err)
	}

	return path
}

func TestTheWorkingConfigLoads(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(write(t, good))
	if err != nil {
		t.Fatalf("a valid config was refused: %v", err)
	}

	if cfg.Database.MaxConns != 80 {
		t.Errorf("max_conns = %d, want 80", cfg.Database.MaxConns)
	}

	if _, ok := cfg.Admins["someone"]; !ok {
		t.Error("the admins block was not read")
	}
}

// The one the repository's own file got wrong. scfgs comments start
// with '#'; a line starting with '//' is a directive named "//", and
// was silently ignored — so was anything commented out that way.
func TestASlashSlashCommentIsRefusedWithAnExplanation(t *testing.T) {
	t.Parallel()

	body := strings.Replace(good, "\tmax_conns 80\n",
		"\t// this is not a comment\n\tmax_conns 80\n", 1)

	_, err := config.Load(write(t, body))
	if err == nil {
		t.Fatal("a '//' line was accepted as a comment")
	}

	if !errors.Is(err, config.ErrUnknownDirective) {
		t.Errorf("error = %v, want an unknown-directive error", err)
	}

	if !strings.Contains(err.Error(), "#") {
		t.Errorf("the message does not say what a comment looks like: %v", err)
	}
}

func TestAHashCommentIsFine(t *testing.T) {
	t.Parallel()

	body := strings.Replace(good, "\tmax_conns 80\n",
		"\t# this is a comment\n\tmax_conns 80\n", 1)

	if _, err := config.Load(write(t, body)); err != nil {
		t.Errorf("a '#' comment was refused: %v", err)
	}
}

// A misspelt directive is indistinguishable from an absent one once
// the file is parsed, and an absent one is a zero — which is a working
// value for most of these. Reporting it is the only place it can be
// caught.
func TestAMisspeltDirectiveIsRefused(t *testing.T) {
	t.Parallel()

	body := strings.Replace(good, "max_conns 80", "max_conn 80", 1)

	_, err := config.Load(write(t, body))
	if err == nil {
		t.Fatal("a misspelt directive was accepted")
	}

	if !strings.Contains(err.Error(), "max_conn") {
		t.Errorf("the message does not name the directive: %v", err)
	}
}

func TestSettingsThatCannotBeAbsentAreChecked(t *testing.T) {
	t.Parallel()

	for _, missing := range []struct {
		name string
		line string
		bare string
	}{
		{"database.url", "url postgres:///cca", `url ""`},
		{"listen.address", "address :8080", `address ""`},
		{
			"session.key",
			"key 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			`key ""`,
		},
		{"oidc.client", "client c", `client ""`},
		{"oidc.jwks", "jwks https://example.invalid/keys", `jwks ""`},
		{"admins", "\tsomeone\n", ""},
	} {
		body := strings.Replace(good, missing.line, missing.bare, 1)

		_, err := config.Load(write(t, body))
		if err == nil {
			t.Errorf("a config with no %s was accepted", missing.name)

			continue
		}

		if !errors.Is(err, config.ErrMissing) {
			t.Errorf("%s: error = %v, want a missing-setting error", missing.name, err)
		}
	}
}

// Durations are nanoseconds, which makes a wrong power of ten easy to
// type and impossible to see: 5000000 beside 5000000000 is five
// milliseconds beside five seconds, and a connect timeout of five
// milliseconds fails under exactly the load it was set for.
func TestADurationOffByAPowerOfTenIsRefused(t *testing.T) {
	t.Parallel()

	body := strings.Replace(good, "connect_timeout 5000000000",
		"connect_timeout 5000000", 1)

	_, err := config.Load(write(t, body))
	if err == nil {
		t.Fatal("a five-millisecond connect timeout was accepted")
	}

	if !errors.Is(err, config.ErrOutOfRange) {
		t.Errorf("error = %v, want an out-of-range error", err)
	}

	if !strings.Contains(err.Error(), "nanoseconds") {
		t.Errorf("the message does not explain the unit: %v", err)
	}
}

func TestAPoolThatCannotServeIsRefused(t *testing.T) {
	t.Parallel()

	for _, broken := range []string{
		"max_conns 0",
		"min_conns 100", // above max_conns
		"min_conns -1",
	} {
		field, _, _ := strings.Cut(broken, " ")
		body := strings.Replace(good, field+" 80", broken, 1)
		body = strings.Replace(body, field+" 10", broken, 1)

		if _, err := config.Load(write(t, body)); err == nil {
			t.Errorf("%q was accepted", broken)
		}
	}
}

// The file shipped in the repository is the one an operator copies, so
// it has to be one the loader accepts — apart from the placeholders it
// carries in place of a deployment's own values, which are shaped so
// that an unedited copy fails at startup rather than running on a
// known session key or somebody else's tenant.
func TestTheShippedConfigParses(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../cca.conf.example")
	if err != nil {
		t.Skipf("no shipped config to check: %v", err)
	}

	if _, err := config.Load(write(t, string(body))); err != nil {
		t.Errorf("the shipped cca.conf.example does not load: %v", err)
	}
}
