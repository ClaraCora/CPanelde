package singbox

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/kernel"
	"github.com/ClaraCora/CPanelde/internal/model"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/service"
	"golang.org/x/net/proxy"
)

func TestReloadAppliesOutboundEndpointAndFinalRoute(t *testing.T) {
	listen := func() (net.Listener, int) {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		return listener, listener.Addr().(*net.TCPAddr).Port
	}
	first, firstPort := listen()
	second, secondPort := listen()
	hit := make(chan int, 4)
	for index, listener := range []net.Listener{first, second} {
		go func(index int, listener net.Listener) {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				hit <- index
				_ = conn.Close()
			}
		}(index, listener)
	}
	probe, nodePort := listen()
	_ = probe.Close()
	node := &model.NodeSpec{Protocol: "socks", ListenIP: "127.0.0.1", ServerPort: nodePort, DefaultOutboundTag: "direct",
		CustomOutbounds: []model.OutboundConfig{{Tag: "gateway", Protocol: "socks", Settings: map[string]any{"server": "127.0.0.1", "server_port": firstPort, "version": "5"}}},
	}
	users := []model.UserSpec{{ID: 1, UUID: "test-user"}}
	k := New(config.KernelConfig{Type: "singbox", LogLevel: "error"})
	if err := k.Start(node, users, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(k.Stop)
	next := *node
	next.DefaultOutboundTag = "gateway"
	next.CustomOutbounds = []model.OutboundConfig{{Tag: "gateway", Protocol: "socks", Settings: map[string]any{"server": "127.0.0.1", "server_port": secondPort, "version": "5"}}}
	if err := k.Reload(&next, users, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	// Route an actual inbound request. The test upstream deliberately closes
	// after accept; reaching the new endpoint proves both route.final and the
	// existing outbound tag's settings were replaced in the running kernel.
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(nodePort)), &proxy.Auth{User: "test-user", Password: "test-user"}, &net.Dialer{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	assertNewEndpoint := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, _ := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", "8.8.8.8:80")
		if conn != nil {
			_ = conn.Close()
		}
		select {
		case index := <-hit:
			if index != 1 {
				t.Fatal("runtime used the wrong outbound endpoint")
			}
		case <-ctx.Done():
			t.Fatal("runtime did not apply the new final outbound")
		}
	}
	assertNewEndpoint()
	// The first replacement succeeds, then the second outbound fails at
	// construction. Restoring the old nodeConfig pointer alone is insufficient:
	// the existing manager's endpoint and routing must also be rolled back.
	rejected := next
	rejected.CustomOutbounds = []model.OutboundConfig{
		{Tag: "gateway", Protocol: "socks", Settings: map[string]any{"server": "127.0.0.1", "server_port": firstPort, "version": "5"}},
		{Tag: "broken", Protocol: "socks", Settings: map[string]any{"server": "127.0.0.1", "server_port": firstPort, "version": "invalid"}},
	}
	if err := k.Reload(&rejected, users, kernel.TLSCert{}); err == nil {
		t.Fatal("partial outbound failure was reported applied")
	}
	if k.nodeConfig != &next {
		t.Fatal("failed reload committed rejected config")
	}
	assertNewEndpoint()
	withoutGateway := next
	withoutGateway.CustomOutbounds = nil
	withoutGateway.DefaultOutboundTag = "direct"
	if err := k.Reload(&withoutGateway, users, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	manager := service.FromContext[adapter.OutboundManager](k.ctx)
	if _, exists := manager.Outbound("gateway"); exists {
		t.Fatal("removed outbound remained active")
	}
}

func TestReloadFailureDoesNotCommitKernelConfiguration(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	node := &model.NodeSpec{Protocol: "socks", ListenIP: "127.0.0.1", ServerPort: port}
	users := []model.UserSpec{{ID: 1, UUID: "test-user"}}
	k := New(config.KernelConfig{Type: "singbox", LogLevel: "error"})
	if err = k.Start(node, users, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(k.Stop)
	next := *node
	next.CustomOutbounds = []model.OutboundConfig{{Tag: "invalid", Protocol: "not-a-protocol"}}
	if err = k.Reload(&next, users, kernel.TLSCert{}); err == nil {
		t.Fatal("unsupported runtime config was claimed applied")
	}
	if k.nodeConfig != node || !k.IsRunning() {
		t.Fatal("failed reload replaced the last good runtime")
	}
}

func TestReloadRebindsChainBeforeRemovingOldUpstream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	node := &model.NodeSpec{Protocol: "socks", ListenIP: "127.0.0.1", ServerPort: port, CustomOutbounds: []model.OutboundConfig{
		{Tag: "upstream-a", Protocol: "direct"},
		{Tag: "upstream-b", Protocol: "direct"},
		{Tag: "chain", Protocol: "socks", ProxyTag: "upstream-a", Settings: map[string]any{"server": "127.0.0.1", "server_port": 1080}},
	}}
	users := []model.UserSpec{{ID: 1, UUID: "test-user"}}
	k := New(config.KernelConfig{Type: "singbox", LogLevel: "error"})
	if err = k.Start(node, users, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(k.Stop)
	next := *node
	next.CustomOutbounds = []model.OutboundConfig{
		{Tag: "chain", Protocol: "socks", ProxyTag: "upstream-b", Settings: map[string]any{"server": "127.0.0.1", "server_port": 1080}},
		{Tag: "upstream-b", Protocol: "direct"},
	}
	if err = k.Reload(&next, users, kernel.TLSCert{}); err != nil {
		t.Fatal(err)
	}
	manager := service.FromContext[adapter.OutboundManager](k.ctx)
	if _, exists := manager.Outbound("upstream-a"); exists {
		t.Fatal("old chained upstream was not removed")
	}
	chain, exists := manager.Outbound("chain")
	if !exists || len(chain.Dependencies()) != 1 || chain.Dependencies()[0] != "upstream-b" {
		t.Fatal("running chain retained its old upstream")
	}
}
