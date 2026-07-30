package platformclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestClientUsesCACCRoutesAndBearerAuthentication(t *testing.T) {
	var mu sync.Mutex
	requests := make([]string, 0, 7)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agent-secret" {
			t.Errorf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Corade-Protocol-Version") != ProtocolVersion {
			t.Errorf("unexpected protocol header %q", r.Header.Get("X-Corade-Protocol-Version"))
		}
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()

		var data any = map[string]any{"accepted": true}
		switch r.URL.Path {
		case "/ca/cc/ws":
			data = map[string]any{"protocol_version": ProtocolVersion, "machine": map[string]any{"id": "mch_test"}}
		case "/ca/cc/fwq/jd":
			data = map[string]any{"nodes": []any{map[string]any{"id": 12, "type": "vless", "name": "edge"}}}
		case "/ca/cc/jd/12/pz":
			data = map[string]any{"node_id": 12, "revision": 3, "protocol": "vless", "listen_ip": "0.0.0.0", "server_port": 443, "kernel_type": "singbox", "settings": map[string]any{"transport": "tcp"}}
		case "/ca/cc/jd/12/yh":
			data = map[string]any{"users": []any{map[string]any{"id": 9, "uuid": "user-uuid"}}}
		case "/ca/cc/bg":
			data = map[string]any{"changes": []any{}, "next_cursor": "8"}
		case "/ca/cc/fwq/xt":
			data = map[string]any{"accepted": true, "commands": []any{map[string]any{"id": "upg_test", "type": "agent.upgrade"}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"request_id": "req_test"}, "error": nil})
	}))
	defer server.Close()

	client := New(server.URL, "agent-secret")
	ctx := context.Background()
	if _, err := client.Handshake(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Nodes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.NodeSpec(ctx, 12); err != nil {
		t.Fatal(err)
	}
	if _, err := client.NodeUsers(ctx, 12); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Changes(ctx, "7"); err != nil {
		t.Fatal(err)
	}
	heartbeat, err := client.SendHeartbeat(ctx, Heartbeat{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !heartbeat.Accepted || len(heartbeat.Commands) != 1 || heartbeat.Commands[0].Type != "agent.upgrade" {
		t.Fatalf("unexpected heartbeat response: %+v", heartbeat)
	}
	if err := client.SendTelemetry(ctx, "batch-1", TelemetryBatch{}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"POST /ca/cc/ws", "GET /ca/cc/fwq/jd", "GET /ca/cc/jd/12/pz", "GET /ca/cc/jd/12/yh",
		"GET /ca/cc/bg?yb=7", "POST /ca/cc/fwq/xt", "POST /ca/cc/yc",
	}
	if len(requests) != len(want) {
		t.Fatalf("got requests %v", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request %d = %q, want %q", i, requests[i], want[i])
		}
	}
}

func TestClientReturnsStructuredPlatformError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": nil, "error": map[string]any{"code": "AGENT_TOKEN_INVALID", "message": "invalid"}})
	}))
	defer server.Close()

	_, err := New(server.URL, "bad").Nodes(context.Background())
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusUnauthorized || apiErr.Code != "AGENT_TOKEN_INVALID" {
		t.Fatalf("unexpected error %#v", err)
	}
}
