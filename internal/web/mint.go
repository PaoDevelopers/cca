package web

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PaoDevelopers/cca/internal/config"
)

// MintSession produces the cookie value for "<role>:<subject>" under
// the configured signing key, without starting a server or opening a
// database.
//
// It exists so that anything needing a session cookie outside a
// browser — the end-to-end tests, or an administrator debugging an
// account — uses the same encoder the running server does. A test that
// reimplemented the format in its own language would be a second
// implementation of a security-relevant contract, and would keep
// passing after the first one changed.
func MintSession(configPath string, spec string) (string, error) {
	role, subject, ok := strings.Cut(spec, ":")
	if !ok {
		return "", fmt.Errorf("%w: want <role>:<subject>, got %q",
			errBadSessionSpec, spec)
	}

	if role != roleStudent && role != roleAdmin {
		return "", fmt.Errorf("%w: role is %q, want %q or %q",
			errBadSessionSpec, role, roleStudent, roleAdmin)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("load configuration: %w", err)
	}

	key, err := newSessionKey(cfg.Session.Key)
	if err != nil {
		return "", fmt.Errorf("session key: %w", err)
	}

	value, err := key.encodeSession(role, subject, time.Now().Add(sessionLifetime))
	if err != nil {
		return "", fmt.Errorf("mint session: %w", err)
	}

	return value, nil
}

var errBadSessionSpec = errors.New("malformed session specification")
