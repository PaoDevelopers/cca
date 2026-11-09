package web

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// -dev-login lets anyone at the sign-in page name the account they
// want, for running cca on a laptop with no identity provider. It
// skips only the proof of who you are: the address still goes through
// startSession, so the domain rule, the administrator list and the
// student roster all apply exactly as they do to a real sign-in.
//
// Since that proof is the whole of the authentication, the switch
// refuses to start on anything but a loopback listener.

var (
	errDevLoginExposed  = errors.New("-dev-login needs a loopback tcp listen address")
	errDevLoginNoTokens = errors.New("-dev-login accepts no identity provider tokens")
)

func checkLoopback(network string, address string) error {
	if !strings.HasPrefix(network, "tcp") {
		return fmt.Errorf("%w: network is %q", errDevLoginExposed, network)
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %w", errDevLoginExposed, err)
	}

	if host == "localhost" {
		return nil
	}

	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%w: address is %q", errDevLoginExposed, address)
	}

	return nil
}

// No key set is fetched under -dev-login, so no token can verify.
func refuseTokens(*jwt.Token) (any, error) {
	return nil, errDevLoginNoTokens
}

func (app *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleDevLogin")

	boundBodyRead(w, bodyReadTimeout)

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthCallback)

	if err := r.ParseForm(); err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMalformed form", err)

		return
	}

	role := r.PostFormValue("role")
	if role != roleStudent && role != roleAdmin {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMissing or malformed role", nil)

		return
	}

	// A bare localpart gets the domain that role's accounts live on.
	email := strings.TrimSpace(r.PostFormValue("email"))
	if !strings.Contains(email, "@") {
		if role == roleAdmin {
			email += "@ykpaoschool.cn"
		} else {
			email += "@stu.ykpaoschool.cn"
		}
	}

	app.startSession(w, r, role, email)
}
