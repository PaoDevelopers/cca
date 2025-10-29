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
	mu              sync.Mutex
	studentChannels map[int64]map[chan BrokerMsg]struct{}
	chlen           int
}

func NewBroker(chlen int) *Broker {
	return &Broker{
		studentChannels: make(map[int64]map[chan BrokerMsg]struct{}),
		chlen:           chlen,
	}
}

func (b *Broker) Broadcast(msg BrokerMsg) {
	if strings.Contains(msg.event, "\n") || strings.Contains(msg.data, "\n") {
		panic("newlines are not allowed in SSE messages")
	}
	b.mu.Lock()
	targets := make([]chan BrokerMsg, 0)
	for _, subs := range b.studentChannels {
		for ch := range subs {
			targets = append(targets, ch)
		}
	}
	b.mu.Unlock()
	slog.Info("broker broadcast", slog.String("event", msg.event), slog.Int("targets", len(targets)))
	for _, ch := range targets {
		select {
		case ch <- msg:
		default:
			slog.Warn("dropping sse message for slow client")
		}
	}
}

func (b *Broker) SubscribeStudent(studentID int64) chan BrokerMsg {
	ch := make(chan BrokerMsg, b.chlen)
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.studentChannels[studentID]; !ok {
		b.studentChannels[studentID] = make(map[chan BrokerMsg]struct{})
	}
	b.studentChannels[studentID][ch] = struct{}{}
	slog.Info("broker subscribe student", slog.Int64("student_id", studentID), slog.Int("channel_count", len(b.studentChannels[studentID])))
	return ch
}

func (b *Broker) UnsubscribeStudent(studentID int64, ch chan BrokerMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if subs, ok := b.studentChannels[studentID]; ok {
		if _, present := subs[ch]; present {
			delete(subs, ch)
			close(ch)
			if len(subs) == 0 {
				delete(b.studentChannels, studentID)
			}
			slog.Info("broker unsubscribe student", slog.Int64("student_id", studentID), slog.Int("remaining", len(subs)))
		}
	}
}

func (b *Broker) BroadcastToStudents(studentIDs []int64, msg BrokerMsg) {
	if strings.Contains(msg.event, "\n") || strings.Contains(msg.data, "\n") {
		panic("newlines are not allowed in SSE messages")
	}
	b.mu.Lock()
	targets := make([]chan BrokerMsg, 0)
	for _, id := range studentIDs {
		if subs, ok := b.studentChannels[id]; ok {
			for ch := range subs {
				targets = append(targets, ch)
			}
		}
	}
	b.mu.Unlock()
	slog.Info("broker targeted broadcast", slog.String("event", msg.event), slog.Int("targets", len(targets)))
	for _, ch := range targets {
		select {
		case ch <- msg:
		default:
			slog.Warn("dropping targeted sse message for slow client")
		}
	}
}
