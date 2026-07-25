package httpapi

import "git.sr.ht/~runxiyu/cca/internal/realtime"

// WSMessage is a text message delivered over a student WebSocket.
type WSMessage = realtime.WSMessage

// CourseStateUpdate is the latest SQL-backed count and revision for a course.
type CourseStateUpdate = realtime.CourseStateUpdate

// WebSocketHub coordinates connected student clients.
type WebSocketHub = realtime.WebSocketHub

// NewWebSocketHub creates a student WebSocket hub.
var NewWebSocketHub = realtime.NewWebSocketHub
