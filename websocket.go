package main

import (
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

type WSMessage string

type Client struct {
	conn      *websocket.Conn
	send      chan WSMessage
	hub       *WebSocketHub
	studentID int64
}

type WebSocketHub struct {
	clients         map[int64]map[*Client]struct{}
	broadcast       chan WSMessage
	register        chan *Client
	unregister      chan *Client
	mu              sync.RWMutex
	broadcastTarget chan struct {
		studentIDs []int64
		message    WSMessage
	}
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[int64]map[*Client]struct{}),
		broadcast:  make(chan WSMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcastTarget: make(chan struct {
			studentIDs []int64
			message    WSMessage
		}, 256),
	}
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if _, ok := h.clients[client.studentID]; !ok {
				h.clients[client.studentID] = make(map[*Client]struct{})
			}
			h.clients[client.studentID][client] = struct{}{}
			h.mu.Unlock()
			slog.Info("websocket client registered", slog.Int64("student_id", client.studentID))

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.studentID]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.clients, client.studentID)
					}
				}
			}
			h.mu.Unlock()
			slog.Info("websocket client unregistered", slog.Int64("student_id", client.studentID))

		case message := <-h.broadcast:
			h.mu.RLock()
			for _, clients := range h.clients {
				for client := range clients {
					select {
					case client.send <- message:
					default:
						slog.Warn("dropping websocket message for slow client")
					}
				}
			}
			h.mu.RUnlock()
			slog.Info("websocket broadcast", slog.String("message", string(message)))

		case target := <-h.broadcastTarget:
			h.mu.RLock()
			for _, studentID := range target.studentIDs {
				if clients, ok := h.clients[studentID]; ok {
					for client := range clients {
						select {
						case client.send <- target.message:
						default:
							slog.Warn("dropping targeted websocket message for slow client")
						}
					}
				}
			}
			h.mu.RUnlock()
			slog.Info("websocket targeted broadcast", slog.String("message", string(target.message)), slog.Int("targets", len(target.studentIDs)))
		}
	}
}

func (h *WebSocketHub) Broadcast(msg WSMessage) {
	h.broadcast <- msg
}

func (h *WebSocketHub) BroadcastToStudents(studentIDs []int64, msg WSMessage) {
	h.broadcastTarget <- struct {
		studentIDs []int64
		message    WSMessage
	}{studentIDs: studentIDs, message: msg}
}

func (c *Client) writePump() {
	defer func() {
		_ = c.conn.Close()
	}()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			slog.Error("websocket write error", slog.Any("error", err))
			return
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("websocket read error", slog.Any("error", err))
			}
			break
		}
	}
}
