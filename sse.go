package main

import "sync"

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
	return ch
}

func (b *Broker) Unsubscribe(ch chan BrokerMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *Broker) Broadcast(msg BrokerMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// TODO: Disconnect that client, or something...
		}
	}
}
