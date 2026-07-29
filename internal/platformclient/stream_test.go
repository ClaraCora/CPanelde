package platformclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStreamUsesBearerHeaderAndDeliversEnvelope(t *testing.T) {
	authorized := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorized <- r.Header.Get("Authorization") == "Bearer secret"
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(Event{ID: "evt_1", Type: "node.spec.replace", Revision: 4, Data: json.RawMessage(`{"node_id":12}`)})
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	events := make(chan Event, 1)
	statuses := make(chan bool, 2)
	stream := NewStream("ws"+strings.TrimPrefix(server.URL, "http"), "secret", 10*time.Millisecond, 20*time.Millisecond,
		func(event Event) { events <- event }, func(connected bool) { statuses <- connected })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stream.Run(ctx)

	select {
	case ok := <-authorized:
		if !ok {
			t.Fatal("stream did not use bearer authentication")
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not connect")
	}
	select {
	case event := <-events:
		if event.NodeID() != 12 || event.Revision != 4 {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("stream event not delivered")
	}
	if !stream.IsConnected() {
		t.Fatal("stream should be connected")
	}
	cancel()
}
