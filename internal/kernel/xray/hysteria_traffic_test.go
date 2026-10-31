package xray

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/kernel"
	"github.com/ClaraCora/CPanelde/internal/model"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol/tls/cert"
	xrayCore "github.com/xtls/xray-core/core"
	xraystats "github.com/xtls/xray-core/features/stats"
)

// Exercise real QUIC authentication and the inbound stats wrapper. Registering
// counters by hand would miss a lost transport user before DispatchLink.
func TestHysteriaUserTrafficWithInboundStats(t *testing.T) {
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			destination := startHysteriaEcho(t, network)
			portProbe, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := portProbe.LocalAddr().(*net.UDPAddr).Port
			portProbe.Close()

			certificate, digest := cert.MustGenerate(nil, cert.DNSNames("localhost"))
			certPEM, keyPEM := certificate.ToPEM()
			users := []model.UserSpec{
				{ID: 101, UUID: "hy2-traffic-test-user", DeviceLimit: 1},
				{ID: 102, UUID: "hy2-inactive-test-user"},
			}
			// Permit only the loopback echo fixture ahead of the default private-IP block.
			customConfig := filepath.Join(t.TempDir(), "loopback.json")
			if err := os.WriteFile(customConfig, []byte(`{"routing":{"rules":[{"type":"field","ip":["127.0.0.1/32"],"outboundTag":"direct"}]}}`), 0600); err != nil {
				t.Fatal(err)
			}
			server := New(config.KernelConfig{Type: "xray", LogLevel: "error", CustomConfig: customConfig})
			if err := server.Start(&model.NodeSpec{
				Protocol: "hysteria", Version: 2, ListenIP: "127.0.0.1", ServerPort: port,
			}, users, kernel.TLSCert{CertPEM: certPEM, KeyPEM: keyPEM}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(server.Stop)

			clientConfig, err := json.Marshal(M{
				"log": M{"loglevel": "error"},
				"outbounds": []M{{
					"protocol": "hysteria",
					"settings": M{"version": 2, "address": "127.0.0.1", "port": port},
					"streamSettings": M{
						"network":          "hysteria",
						"hysteriaSettings": M{"version": 2, "auth": users[0].UUID, "udpIdleTimeout": 60},
						"security":         "tls",
						"tlsSettings": M{
							"serverName": "localhost", "alpn": []string{"h3"},
							"pinnedPeerCertSha256": hex.EncodeToString(digest[:]),
						},
					},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			client, err := xrayCore.StartInstance("json", clientConfig)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { client.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			conn, err := xrayCore.Dial(ctx, client, destination)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { conn.Close() })
			payload := bytes.Repeat([]byte("hy2-accounting-"), 32)
			reply := make([]byte, len(payload))
			done := make(chan error, 1)
			go func() {
				if _, err := conn.Write(payload); err != nil {
					done <- err
					return
				}
				_, err := io.ReadFull(conn, reply)
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("HY2 echo: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("HY2 echo timed out")
			}
			if !bytes.Equal(reply, payload) {
				t.Fatal("HY2 echo payload mismatch")
			}

			traffic, aliveIPs, connections, err := server.GetUserTraffic(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := [2]int64{int64(len(payload)), int64(len(payload))}
			if got := traffic[users[0].ID]; got != want {
				t.Errorf("authenticated user traffic = %v, want %v", got, want)
			}
			if got := traffic[users[1].ID]; got != [2]int64{} {
				t.Errorf("inactive user traffic = %v, want zero", got)
			}
			if connections < 1 {
				t.Errorf("active user connections = %d, want at least 1", connections)
			}
			if network == "tcp" && !aliveIPs[users[0].ID]["127.0.0.1"] {
				t.Error("authenticated user's active IP is missing")
			}
			trafficAgain, _, _, err := server.GetUserTraffic(ctx)
			if err != nil || trafficAgain[users[0].ID] != want {
				t.Errorf("second traffic poll = %v, %v; want unchanged cumulative bytes", trafficAgain, err)
			}
			// User accounting must coexist with system inbound accounting.
			stats := server.instance.GetFeature(xraystats.ManagerType()).(xraystats.Manager)
			for _, direction := range []string{"uplink", "downlink"} {
				counter := stats.GetCounter(fmt.Sprintf("inbound>>>hysteria-in>>>traffic>>>%s", direction))
				if counter == nil || counter.Value() < int64(len(payload)) {
					t.Errorf("system inbound %s counter is missing or too small", direction)
				}
			}
		})
	}
}

func startHysteriaEcho(t *testing.T, network string) xnet.Destination {
	t.Helper()
	var address net.Addr
	if network == "tcp" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { listener.Close() })
		address = listener.Addr()
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(15 * time.Second))
			io.Copy(conn, conn)
		}()
	} else {
		listener, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { listener.Close() })
		address = listener.LocalAddr()
		go func() {
			b := make([]byte, 2048)
			for {
				n, remote, err := listener.ReadFrom(b)
				if err != nil {
					return
				}
				if _, err := listener.WriteTo(b[:n], remote); err != nil {
					return
				}
			}
		}()
	}
	destination, err := xnet.ParseDestination(network + ":" + address.String())
	if err != nil {
		t.Fatal(err)
	}
	return destination
}
