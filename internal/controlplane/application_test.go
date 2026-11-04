package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/platformclient"
)

func TestSnapshotFailureReportsStageWithoutAcknowledging(t *testing.T) {
	for _, stage := range []string{"convert", "users"} {
		t.Run(stage, func(t *testing.T) {
			var results []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data any
				switch r.URL.Path {
				case "/ca/cc/jd/12/pz":
					settings := any(map[string]any{"transport": "tcp"})
					if stage == "convert" {
						settings = "invalid settings object"
					}
					data = map[string]any{"node_id": 12, "revision": 3, "protocol": "vless", "server_port": 443, "kernel_type": "xray", "settings": settings}
				case "/ca/cc/jd/12/yh":
					w.WriteHeader(http.StatusServiceUnavailable)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "UNAVAILABLE", "message": "users unavailable"}})
					return
				case "/ca/cc/yc":
					var batch platformclient.TelemetryBatch
					if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
						t.Error(err)
					}
					if len(batch.Events) == 1 && batch.Events[0].Type == "node.application" {
						results = append(results, batch.Events[0].Data)
					}
					data = map[string]bool{"accepted": true}
				default:
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "error": nil})
			}))
			defer server.Close()
			cp := NewDevicePlatformControlPlane(platformclient.New(server.URL, "token"), 12, config.KernelConfig{Type: "xray"}, 60, 60, nil, nil)
			if _, err := cp.Poll(context.Background()); err == nil {
				t.Fatal("expected snapshot failure")
			}
			if cp.revision.Load() != 0 {
				t.Fatal("failed snapshot acknowledged revision")
			}
			if len(results) != 2 || results[0]["status"] != "received" || results[1]["status"] != "failed" || results[1]["stage"] != stage {
				t.Fatalf("results=%+v", results)
			}
		})
	}
}

func TestApplicationResultRetainsKeyAndPayloadAfterLostResponse(t *testing.T) {
	var keys, bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		bodies = append(bodies, string(body))
		if len(keys) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]bool{"accepted": true}, "error": nil})
	}))
	defer server.Close()
	cp := NewDevicePlatformControlPlane(platformclient.New(server.URL, "token"), 12, config.KernelConfig{Type: "xray"}, 60, 60, nil, nil)
	cp.ReportApplication(ApplicationResult{Revision: 3, Status: "applied", Stage: "kernel"})
	if len(cp.pendingApplications) != 1 {
		t.Fatal("unconfirmed application result was discarded")
	}
	cp.flushApplications()
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] || bodies[0] != bodies[1] {
		t.Fatal("application retry identity or body changed")
	}
	if len(cp.pendingApplications) != 0 {
		t.Fatal("confirmed result remains queued")
	}
}
