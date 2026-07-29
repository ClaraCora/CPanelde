package platformclient

import (
	"encoding/json"
	"testing"

	"github.com/ClaraCora/CPanelde/internal/config"
)

func TestNodeSpecModelNormalizesPlatformSettings(t *testing.T) {
	spec := NodeSpec{
		NodeID: 12, Revision: 4, Protocol: "vless", ListenIP: "127.0.0.1", ServerPort: 443, KernelType: "singbox",
		Settings: json.RawMessage(`{
			"transport":"ws",
			"network_settings":{"path":"/edge","host":"edge.example.com"},
			"tls":{"enabled":true,"server_name":"edge.example.com"},
			"flow":"xtls-rprx-vision"
		}`),
	}

	modelSpec, err := spec.Model(config.KernelConfig{Type: "singbox"})
	if err != nil {
		t.Fatal(err)
	}
	if modelSpec.Protocol != "vless" || modelSpec.ListenIP != "127.0.0.1" || modelSpec.ServerPort != 443 {
		t.Fatalf("unexpected identity: %+v", modelSpec)
	}
	if modelSpec.Network != "ws" || modelSpec.NetworkSettings["path"] != "/edge" {
		t.Fatalf("unexpected transport: %+v", modelSpec)
	}
	if modelSpec.TLS != 1 || modelSpec.TLSSettings["server_name"] != "edge.example.com" {
		t.Fatalf("unexpected tls: %+v", modelSpec)
	}
	if modelSpec.Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected flow %q", modelSpec.Flow)
	}
}

func TestUsersResponseModels(t *testing.T) {
	users := (UsersResponse{Users: []User{{ID: 7, UUID: "uuid", SpeedLimit: 20, DeviceLimit: 2}}}).Models()
	if len(users) != 1 || users[0].ID != 7 || users[0].SpeedLimit != 20 || users[0].DeviceLimit != 2 {
		t.Fatalf("unexpected users: %+v", users)
	}
}
