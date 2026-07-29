package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/platformclient"
)

func TestDevicePlatformControlPlaneSnapshotAndReports(t *testing.T) {
	var mu sync.Mutex
	paths := make([]string, 0, 6)
	telemetryTypes := make([]string, 0, 2)
	var reportedRevision any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()

		var data any
		switch r.URL.Path {
		case "/ca/cc/jd/12/pz":
			data = map[string]any{
				"node_id": 12, "revision": 5, "protocol": "vless", "listen_ip": "0.0.0.0",
				"server_port": 443, "kernel_type": "singbox", "settings": map[string]any{"transport": "tcp"},
			}
		case "/ca/cc/jd/12/yh":
			data = map[string]any{"users": []any{map[string]any{
				"id": 9, "uuid": "11111111-1111-1111-1111-111111111111", "speed_limit": 20, "device_limit": 2,
			}}}
		case "/ca/cc/yc":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Error("missing idempotency key")
			}
			var batch platformclient.TelemetryBatch
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				t.Errorf("decode telemetry: %v", err)
			} else if len(batch.Events) != 1 {
				t.Errorf("events = %d, want 1", len(batch.Events))
			} else {
				mu.Lock()
				telemetryTypes = append(telemetryTypes, batch.Events[0].Type)
				if batch.Events[0].Type == "node.telemetry" {
					reportedRevision = batch.Events[0].Data["revision"]
				}
				mu.Unlock()
			}
			data = map[string]bool{"accepted": true}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "error": nil})
	}))
	defer server.Close()

	registered := false
	client := platformclient.New(server.URL, "agent-token")
	cp := NewDevicePlatformControlPlane(client, 12, config.KernelConfig{Type: "singbox"}, 45, 90, nil,
		func(statuses chan<- StatusChange) *NodeMailbox {
			registered = statuses != nil
			return nil
		})
	statuses := make(chan StatusChange, 1)
	bootstrap, err := cp.Initial(context.Background(), nil, nil, statuses)
	if err != nil {
		t.Fatal(err)
	}
	if !registered || bootstrap.PushInterval != 45 || bootstrap.PullInterval != 90 {
		t.Fatalf("unexpected bootstrap: registered=%v bootstrap=%+v", registered, bootstrap)
	}
	if bootstrap.Config == nil || bootstrap.Config.Protocol != "vless" || bootstrap.Config.ServerPort != 443 {
		t.Fatalf("unexpected node config: %+v", bootstrap.Config)
	}
	if len(bootstrap.Users) != 1 || bootstrap.Users[0].ID != 9 {
		t.Fatalf("unexpected users: %+v", bootstrap.Users)
	}
	if _, err := cp.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := cp.Report(ReportPayload{Traffic: map[int][2]int64{9: {100, 200}}}); err != nil {
		t.Fatal(err)
	}
	cp.ReportDevices(nil, map[int][]string{9: {"192.0.2.10"}})

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 6 {
		t.Fatalf("paths = %v", paths)
	}
	if len(telemetryTypes) != 2 || telemetryTypes[0] != "node.telemetry" || telemetryTypes[1] != "node.devices" {
		t.Fatalf("telemetry types = %v", telemetryTypes)
	}
	if reportedRevision != float64(5) {
		t.Fatalf("reported revision = %#v, want 5", reportedRevision)
	}
}

func TestDevicePlatformControlPlaneRejectsMismatchedNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"node_id": 99, "protocol": "vless", "server_port": 443, "kernel_type": "singbox", "settings": map[string]any{"transport": "tcp"},
		}, "error": nil})
	}))
	defer server.Close()

	cp := NewDevicePlatformControlPlane(platformclient.New(server.URL, "token"), 12, config.KernelConfig{Type: "singbox"}, 60, 60, nil, nil)
	if _, err := cp.Poll(context.Background()); err == nil {
		t.Fatal("expected mismatched node error")
	}
}
