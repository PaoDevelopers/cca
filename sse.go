package main

import (
	"log/slog"
	"strings"
	"sync"
)

type BrokerMsg struct {
	event string
	data  string
}

type Broker struct {
	mu      sync.Mutex
	clients map[chan BrokerMsg]struct{}
	chlen   int
}

func NewBroker(chlen int) *Broker {
	return &Broker{
		clients: make(map[chan BrokerMsg]struct{}),
		chlen:   chlen,
	}
}

func (b *Broker) Subscribe() chan BrokerMsg {
	ch := make(chan BrokerMsg, b.chlen)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[ch] = struct{}{}
	slog.Info("broker subscribe", slog.Int("client_count", len(b.clients)))
	return ch
}

func (b *Broker) Unsubscribe(ch chan BrokerMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
	slog.Info("broker unsubscribe", slog.Int("client_count", len(b.clients)))
}

func (b *Broker) Broadcast(msg BrokerMsg) {
	if strings.Contains(msg.event, "\n") || strings.Contains(msg.data, "\n") {
		panic("newlines are not allowed in SSE messages")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	slog.Info("broker broadcast", slog.String("event", msg.event))
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			slog.Warn("dropping sse message for slow client")
		}
	}
}
