package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	ircServerAddress = "irc.runxiyu.org:6697"
	ircServerName    = "irc.runxiyu.org"
	ircNickname      = "tickets"
	ircChannel       = "#cca"
)

type ircMessage struct {
	Prefix   string
	Command  string
	Params   []string
	Trailing string
}

func runIRC(ctx context.Context, logger *slog.Logger, incoming <-chan string) {
	var queue []string
	for {
		if ctx.Err() != nil {
			return
		}

		conn, err := dialIRC(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("irc dial failed", "error", err)
			drainIncoming(ctx, incoming, &queue)
			if !waitOrDrain(ctx, incoming, &queue, 5*time.Second) {
				return
			}
			continue
		}

		if err := serveIRCConnection(ctx, conn, logger, incoming, &queue); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			logger.Error("irc connection ended", "error", err)
			_ = conn.Close()
			if !waitOrDrain(ctx, incoming, &queue, 5*time.Second) {
				return
			}
			continue
		}

		_ = conn.Close()
		return
	}
}

func dialIRC(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 15 * time.Second,
	}
	rawConn, err := dialer.DialContext(ctx, "tcp", ircServerAddress)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         ircServerName,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	})

	handshakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

func serveIRCConnection(ctx context.Context, conn net.Conn, logger *slog.Logger, incoming <-chan string, queue *[]string) error {
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	var writerMu sync.Mutex
	sendLine := func(line string) error {
		writerMu.Lock()
		defer writerMu.Unlock()
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return err
		}
		return writer.Flush()
	}

	if err := sendLine(fmt.Sprintf("NICK %s", ircNickname)); err != nil {
		return err
	}
	if err := sendLine(fmt.Sprintf("USER %s 0 * :%s", ircNickname, ircNickname)); err != nil {
		return err
	}

	joinedCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			msg, err := parseIRCMessage(line)
			if err != nil {
				logger.Warn("irc parse error", "line", line, "error", err)
				continue
			}
			switch msg.Command {
			case "PING":
				payload := formatIRCParams(msg.Params, msg.Trailing)
				if payload == "" {
					payload = ":" + ircServerName
				}
				if err := sendLine("PONG " + payload); err != nil {
					errCh <- err
					return
				}
			case "001":
				if err := sendLine("JOIN " + ircChannel); err != nil {
					errCh <- err
					return
				}
			case "366":
				if len(msg.Params) >= 2 && strings.EqualFold(msg.Params[1], ircChannel) {
					select {
					case joinedCh <- struct{}{}:
					default:
					}
				}
			case "433":
				errCh <- fmt.Errorf("nickname %q already in use", ircNickname)
				return
			case "JOIN":
				if isOwnJoinMessage(msg) {
					select {
					case joinedCh <- struct{}{}:
					default:
					}
				}
			case "ERROR":
				errCh <- errors.New(messageDescription(msg))
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
			return
		}
		errCh <- io.EOF
	}()

	joined := false
	var joinWait <-chan struct{} = joinedCh

	for {
		if ctx.Err() != nil {
			_ = sendLine("QUIT :shutting down")
			return context.Canceled
		}

		if joined && len(*queue) > 0 {
			next := sanitizeIRCMessage((*queue)[0])
			if err := sendLine(fmt.Sprintf("PRIVMSG %s :%s", ircChannel, next)); err != nil {
				return err
			}
			*queue = (*queue)[1:]
			continue
		}

		select {
		case <-ctx.Done():
			_ = sendLine("QUIT :shutting down")
			return context.Canceled
		case err := <-errCh:
			return err
		case <-joinWait:
			joined = true
			joinWait = nil
		case msg, ok := <-incoming:
			if !ok {
				_ = sendLine("QUIT :no more messages")
				return nil
			}
			*queue = append(*queue, msg)
		}
	}
}

func sanitizeIRCMessage(message string) string {
	replacer := strings.NewReplacer("\r", " ", "\n", " ")
	return strings.TrimSpace(replacer.Replace(message))
}

func parseIRCMessage(line string) (ircMessage, error) {
	var msg ircMessage
	text := strings.TrimSpace(line)
	if text == "" {
		return msg, errors.New("empty irc line")
	}

	if text[0] == ':' {
		space := strings.IndexByte(text, ' ')
		if space <= 1 {
			return msg, fmt.Errorf("malformed prefix: %q", line)
		}
		msg.Prefix = text[1:space]
		text = strings.TrimLeft(text[space+1:], " ")
	}
	if text == "" {
		return msg, fmt.Errorf("missing command: %q", line)
	}

	space := strings.IndexByte(text, ' ')
	if space == -1 {
		msg.Command = strings.ToUpper(text)
		return msg, nil
	}

	msg.Command = strings.ToUpper(text[:space])
	text = strings.TrimLeft(text[space+1:], " ")

	for len(text) > 0 {
		if text[0] == ':' {
			msg.Trailing = text[1:]
			break
		}
		nextSpace := strings.IndexByte(text, ' ')
		if nextSpace == -1 {
			msg.Params = append(msg.Params, text)
			break
		}
		msg.Params = append(msg.Params, text[:nextSpace])
		text = strings.TrimLeft(text[nextSpace+1:], " ")
	}
	return msg, nil
}

func formatIRCParams(params []string, trailing string) string {
	switch {
	case len(params) == 0 && trailing == "":
		return ""
	case trailing == "":
		return strings.Join(params, " ")
	case len(params) == 0:
		return ":" + trailing
	default:
		return strings.Join(params, " ") + " :" + trailing
	}
}

func isOwnJoinMessage(msg ircMessage) bool {
	if msg.Command != "JOIN" {
		return false
	}
	channel := ""
	if msg.Trailing != "" {
		channel = msg.Trailing
	} else if len(msg.Params) > 0 {
		channel = msg.Params[0]
	}
	if !strings.EqualFold(channel, ircChannel) {
		return false
	}
	nick := msg.Prefix
	if nick == "" {
		return false
	}
	if idx := strings.IndexByte(nick, '!'); idx >= 0 {
		nick = nick[:idx]
	}
	return strings.EqualFold(nick, ircNickname)
}

func messageDescription(msg ircMessage) string {
	if msg.Trailing != "" {
		return msg.Trailing
	}
	if len(msg.Params) > 0 {
		return strings.Join(msg.Params, " ")
	}
	return msg.Command
}

func waitOrDrain(ctx context.Context, incoming <-chan string, queue *[]string, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	for {
		if ctx.Err() != nil {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		case msg, ok := <-incoming:
			if !ok {
				return false
			}
			*queue = append(*queue, msg)
		}
	}
}

func drainIncoming(ctx context.Context, incoming <-chan string, queue *[]string) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-incoming:
			if !ok {
				return
			}
			*queue = append(*queue, msg)
		default:
			return
		}
	}
}
