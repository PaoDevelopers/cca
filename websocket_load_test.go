package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWebSocketCourseStateFanout1000Clients(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		studentID := int64(1)
		fmt.Sscan(strings.TrimPrefix(r.URL.Path, "/"), &studentID)
		client := &Client{
			conn:      conn,
			send:      make(chan WSMessage, 256),
			hub:       hub,
			studentID: studentID,
		}
		hub.register <- client
		go client.writePump()
		go client.readPump()
	}))
	defer server.Close()

	const clientCount = 1000
	connections := make([]*websocket.Conn, clientCount)
	var connected atomic.Int64
	var connectWG sync.WaitGroup
	connectLimit := make(chan struct{}, 100)
	connectWG.Add(clientCount)
	for i := range clientCount {
		go func() {
			defer connectWG.Done()
			connectLimit <- struct{}{}
			defer func() { <-connectLimit }()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, fmt.Sprintf(
				"%s/%d",
				strings.Replace(server.URL, "http://", "ws://", 1),
				i+1,
			), nil)
			if err != nil {
				t.Errorf("connect client %d: %v", i+1, err)
				return
			}
			connections[i] = conn
			connected.Add(1)
		}()
	}
	connectWG.Wait()
	if got := connected.Load(); got != clientCount {
		t.Fatalf("connected clients = %d, want %d", got, clientCount)
	}
	defer func() {
		for _, conn := range connections {
			if conn != nil {
				conn.CloseNow()
			}
		}
	}()

	var readWG sync.WaitGroup
	readWG.Add(clientCount)
	for i, conn := range connections {
		go func() {
			defer readWG.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, data, err := conn.Read(ctx)
			if err != nil {
				t.Errorf("read client %d: %v", i+1, err)
				return
			}
			var batch courseStateBatch
			if err := json.Unmarshal(data, &batch); err != nil {
				t.Errorf("decode client %d: %v", i+1, err)
				return
			}
			if batch.Type != "course_state" ||
				len(batch.Courses) != 1 ||
				batch.Courses[0].StateRevision != 1000 {
				t.Errorf("client %d received unexpected batch: %+v", i+1, batch)
			}
		}()
	}

	for revision := int64(1); revision <= 1000; revision++ {
		hub.PublishCourseStates([]CourseStateUpdate{{
			CourseID:        "LOAD",
			CurrentStudents: revision,
			StateRevision:   revision,
		}})
	}
	readWG.Wait()
}
