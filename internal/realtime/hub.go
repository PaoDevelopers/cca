package realtime

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

	logMsgWebsocketClientRegistered       = "websocket.client.registered"
	logMsgWebsocketClientUnregistered     = "websocket.client.unregistered"
	logMsgWebsocketBroadcastAll           = "websocket.broadcast.all"
	logMsgWebsocketBroadcastTargeted      = "websocket.broadcast.targeted"
	logMsgWebsocketBroadcastCourseState   = "websocket.broadcast.course_state"
	logMsgWebsocketCourseStateEncodeError = "websocket.course_state.encode_error"
	logMsgWebsocketSlowClientDisconnected = "websocket.slow_client.disconnected"
	logMsgWebsocketPingFailed             = "websocket.ping.failed"
	logMsgWebsocketWriteError             = "websocket.write.error"
	logMsgWebsocketReadError              = "websocket.read.error"
)

// WSMessage is a text message delivered to connected students.
type WSMessage string

// CourseStateUpdate contains a course count at a monotonically increasing revision.
type CourseStateUpdate struct {
	CourseID        string `json:"course_id"`
	CurrentStudents int64  `json:"current_students"`
	StateRevision   int64  `json:"state_revision"`
}

// Client is one connected student's WebSocket session.
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

// WebSocketHub owns connected clients and batches course-state updates.
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

// NewWebSocketHub constructs an empty hub.
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

// Connect registers conn for studentID and starts its read and write pumps.
func (h *WebSocketHub) Connect(conn *websocket.Conn, studentID int64) error {
	client := &Client{
		conn:      conn,
		send:      make(chan WSMessage, 256),
		hub:       h,
		studentID: studentID,
	}
	h.register <- client

	ctx, cancel := context.WithTimeout(context.Background(), websocketWriteTimeout)
	err := conn.Write(ctx, websocket.MessageText, []byte("hello"))
	cancel()
	if err != nil {
		client.unregister()
		_ = conn.CloseNow()
		return err
	}

	go client.writePump()
	go client.readPump()
	return nil
}

// Run processes client and broadcast events until the process exits.
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

// Broadcast queues a message for every connected client.
func (h *WebSocketHub) Broadcast(msg WSMessage) {
	h.broadcast <- msg
}

// BroadcastToStudents queues a message only for the listed students.
func (h *WebSocketHub) BroadcastToStudents(studentIDs []int64, msg WSMessage) {
	h.broadcastTarget <- targetedWSMessage{studentIDs: studentIDs, message: msg}
}

// PublishCourseStates coalesces SQL-backed course states for batched delivery.
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
		_ = client.conn.CloseNow()
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
		_ = c.conn.CloseNow()
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
		_ = c.conn.CloseNow()
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
