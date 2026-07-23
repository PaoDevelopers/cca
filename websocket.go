package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	websocketWriteTimeout    = 5 * time.Second
	websocketPingInterval    = 25 * time.Second
	websocketPingTimeout     = 10 * time.Second
	courseStateFlushInterval = 50 * time.Millisecond
)

type WSMessage string

type CourseStateUpdate struct {
	CourseID        string `json:"course_id"`
	CurrentStudents int64  `json:"current_students"`
	StateRevision   int64  `json:"state_revision"`
}

type Client struct {
	conn           *websocket.Conn
	send           chan WSMessage
	hub            *WebSocketHub
	studentID      int64
	unregisterOnce sync.Once
}

type targetedWSMessage struct {
	studentIDs []int64
	message    WSMessage
}

type courseStateBatch struct {
	Type    string              `json:"type"`
	Courses []CourseStateUpdate `json:"courses"`
}

type WebSocketHub struct {
	// Run owns clients. Other goroutines communicate only through the channels
	// below, so client registration and removal require no shared map lock.
	clients         map[int64]map[*Client]struct{}
	broadcast       chan WSMessage
	register        chan *Client
	unregister      chan *Client
	broadcastTarget chan targetedWSMessage

	// Course updates are last-write-wins by PostgreSQL revision. A one-slot wake
	// channel turns any size mutation burst into one batched WebSocket frame.
	courseStateMu      sync.Mutex
	pendingCourseState map[string]CourseStateUpdate
	courseStateWake    chan struct{}
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:            make(map[int64]map[*Client]struct{}),
		broadcast:          make(chan WSMessage, 256),
		register:           make(chan *Client),
		unregister:         make(chan *Client, 1024),
		broadcastTarget:    make(chan targetedWSMessage, 256),
		pendingCourseState: make(map[string]CourseStateUpdate),
		courseStateWake:    make(chan struct{}, 1),
	}
}

func (h *WebSocketHub) Run() {
	var courseStateFlush <-chan time.Time

	for {
		select {
		case client := <-h.register:
			if _, ok := h.clients[client.studentID]; !ok {
				h.clients[client.studentID] = make(map[*Client]struct{})
			}
			h.clients[client.studentID][client] = struct{}{}
			slog.Debug(logMsgWebsocketClientRegistered, slog.Int64("student_id", client.studentID))

		case client := <-h.unregister:
			if h.removeClient(client) {
				slog.Debug(logMsgWebsocketClientUnregistered, slog.Int64("student_id", client.studentID))
			}

		case message := <-h.broadcast:
			h.broadcastAll(message)
			slog.Info(logMsgWebsocketBroadcastAll, slog.String("message", string(message)))

		case target := <-h.broadcastTarget:
			h.broadcastStudents(target.studentIDs, target.message)
			slog.Info(
				logMsgWebsocketBroadcastTargeted,
				slog.String("message", string(target.message)),
				slog.Int("targets", len(target.studentIDs)),
			)

		case <-h.courseStateWake:
			if courseStateFlush == nil {
				courseStateFlush = time.After(courseStateFlushInterval)
			}

		case <-courseStateFlush:
			states := h.takeCourseStates()
			courseStateFlush = nil
			if len(states) == 0 {
				continue
			}
			payload, err := json.Marshal(courseStateBatch{
				Type:    "course_state",
				Courses: states,
			})
			if err != nil {
				slog.Error(logMsgWebsocketCourseStateEncodeError, slog.Any("error", err))
				continue
			}
			h.broadcastAll(WSMessage(payload))
			slog.Debug(logMsgWebsocketBroadcastCourseState, slog.Int("courses", len(states)))
		}
	}
}

func (h *WebSocketHub) Broadcast(msg WSMessage) {
	h.broadcast <- msg
}

func (h *WebSocketHub) BroadcastToStudents(studentIDs []int64, msg WSMessage) {
	h.broadcastTarget <- targetedWSMessage{studentIDs: studentIDs, message: msg}
}

func (h *WebSocketHub) PublishCourseStates(states []CourseStateUpdate) {
	if len(states) == 0 {
		return
	}

	h.courseStateMu.Lock()
	for _, state := range states {
		if state.CourseID == "" || state.StateRevision <= 0 || state.CurrentStudents < 0 {
			continue
		}
		current, exists := h.pendingCourseState[state.CourseID]
		if !exists || state.StateRevision > current.StateRevision {
			h.pendingCourseState[state.CourseID] = state
		}
	}
	hasPending := len(h.pendingCourseState) > 0
	h.courseStateMu.Unlock()

	if hasPending {
		select {
		case h.courseStateWake <- struct{}{}:
		default:
		}
	}
}

func (h *WebSocketHub) takeCourseStates() []CourseStateUpdate {
	h.courseStateMu.Lock()
	states := make([]CourseStateUpdate, 0, len(h.pendingCourseState))
	for _, state := range h.pendingCourseState {
		states = append(states, state)
	}
	clear(h.pendingCourseState)
	h.courseStateMu.Unlock()

	sort.Slice(states, func(i, j int) bool {
		return states[i].CourseID < states[j].CourseID
	})
	return states
}

func (h *WebSocketHub) broadcastAll(message WSMessage) {
	for studentID, clients := range h.clients {
		for client := range clients {
			if !h.enqueue(client, message) {
				delete(clients, client)
			}
		}
		if len(clients) == 0 {
			delete(h.clients, studentID)
		}
	}
}

func (h *WebSocketHub) broadcastStudents(studentIDs []int64, message WSMessage) {
	seen := make(map[int64]struct{}, len(studentIDs))
	for _, studentID := range studentIDs {
		if _, exists := seen[studentID]; exists {
			continue
		}
		seen[studentID] = struct{}{}

		clients, ok := h.clients[studentID]
		if !ok {
			continue
		}
		for client := range clients {
			if !h.enqueue(client, message) {
				delete(clients, client)
			}
		}
		if len(clients) == 0 {
			delete(h.clients, studentID)
		}
	}
}

func (h *WebSocketHub) enqueue(client *Client, message WSMessage) bool {
	select {
	case client.send <- message:
		return true
	default:
		// A client that cannot keep up must reconnect and fetch a fresh SQL
		// snapshot. Silently dropping one state message can leave it stale forever.
		close(client.send)
		client.conn.CloseNow()
		slog.Warn(
			logMsgWebsocketSlowClientDisconnected,
			slog.Int64("student_id", client.studentID),
		)
		return false
	}
}

func (h *WebSocketHub) removeClient(client *Client) bool {
	clients, ok := h.clients[client.studentID]
	if !ok {
		return false
	}
	if _, exists := clients[client]; !exists {
		return false
	}
	delete(clients, client)
	close(client.send)
	if len(clients) == 0 {
		delete(h.clients, client.studentID)
	}
	return true
}

func (c *Client) unregister() {
	c.unregisterOnce.Do(func() {
		c.hub.unregister <- c
	})
}

func (c *Client) writePump() {
	pingTicker := time.NewTicker(websocketPingInterval)
	defer func() {
		pingTicker.Stop()
		c.unregister()
		c.conn.CloseNow()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), websocketWriteTimeout)
			err := c.conn.Write(ctx, websocket.MessageText, []byte(message))
			cancel()
			if err != nil {
				slog.Error(logMsgWebsocketWriteError, slog.Any("error", err))
				return
			}

		case <-pingTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), websocketPingTimeout)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				slog.Warn(logMsgWebsocketPingFailed, slog.Any("error", err))
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.unregister()
		c.conn.CloseNow()
	}()

	for {
		_, _, err := c.conn.Read(context.Background())
		if err != nil {
			status := websocket.CloseStatus(err)
			if status != websocket.StatusNormalClosure && status != websocket.StatusGoingAway {
				slog.Error(logMsgWebsocketReadError, slog.Any("error", err))
			}
			return
		}
	}
}
