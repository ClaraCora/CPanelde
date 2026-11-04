package singbox

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/kernel"
	"github.com/ClaraCora/CPanelde/internal/model"
	singlog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/naive"
	"github.com/sagernet/sing/common/auth"
	"golang.org/x/net/proxy"
)

// Exercise the actual listener handshake path concurrently with user replacement.
// Kernel-level locking alone cannot protect authentication in listener goroutines.
func TestProxyAuthenticationDuringUserHotSwap(t *testing.T) {
	for _, protocol := range []string{"socks", "http"} {
		t.Run(protocol, func(t *testing.T) {
			target, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			go func() {
				for {
					conn, err := target.Accept()
					if err != nil {
						return
					}
					go func() {
						defer conn.Close()
						_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
						_, _ = io.Copy(conn, conn)
					}()
				}
			}()
			probe, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := probe.Addr().(*net.TCPAddr).Port
			_ = probe.Close()
			address := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
			node := &model.NodeSpec{
				Protocol: protocol, ListenIP: "127.0.0.1", ServerPort: port,
				// Permit only the local test endpoint before the normal private-IP block.
				CustomRoutes: []map[string]any{{"ip_cidr": []string{"127.0.0.1/32"}, "outbound": "direct"}},
			}
			users := func(rotating string) []model.UserSpec {
				return []model.UserSpec{{ID: 1, UUID: "stable-user"}, {ID: 2, UUID: rotating}}
			}
			k := New(config.KernelConfig{Type: "singbox", LogLevel: "error"})
			if err := k.Start(node, users("retired-user"), kernel.TLSCert{}); err != nil {
				t.Fatal(err)
			}
			defer k.Stop()
			connect := func(user string) error {
				var conn net.Conn
				var reader io.Reader
				if protocol == "socks" {
					dialer, err := proxy.SOCKS5("tcp", address, &proxy.Auth{User: user, Password: user}, &net.Dialer{})
					if err != nil {
						return err
					}
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					conn, err = dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", target.Addr().String())
					if err != nil {
						return err
					}
					reader = conn
				} else {
					var err error
					conn, err = net.DialTimeout("tcp", address, 3*time.Second)
					if err != nil {
						return err
					}
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if protocol == "http" {
					credential := base64.StdEncoding.EncodeToString([]byte(user + ":" + user))
					if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", target.Addr(), target.Addr(), credential); err != nil {
						return err
					}
					buffer := bufio.NewReader(conn)
					status, err := buffer.ReadString('\n')
					if err != nil || !strings.Contains(status, " 200 ") {
						return fmt.Errorf("CONNECT rejected: %q (%v)", status, err)
					}
					for {
						line, err := buffer.ReadString('\n')
						if err != nil {
							return err
						}
						if line == "\r\n" {
							break
						}
					}
					reader = buffer
				}
				if _, err := conn.Write([]byte{'x'}); err != nil {
					return err
				}
				var reply [1]byte
				if _, err := io.ReadFull(reader, reply[:]); err != nil {
					return err
				}
				if reply[0] != 'x' {
					return fmt.Errorf("wrong tunneled payload: %q", reply)
				}
				return nil
			}
			start := make(chan struct{})
			failures := make(chan error, 5)
			var work sync.WaitGroup
			work.Add(1)
			go func() {
				defer work.Done()
				<-start
				for index := range 200 {
					if _, _, err := k.UpdateUsers(users(fmt.Sprintf("rotating-%d", index%2))); err != nil {
						failures <- err
						return
					}
				}
			}()
			for range 4 {
				work.Add(1)
				go func() {
					defer work.Done()
					<-start
					for range 25 {
						if err := connect("stable-user"); err != nil {
							failures <- err
							return
						}
					}
				}()
			}
			close(start)
			work.Wait()
			close(failures)
			for err := range failures {
				t.Error(err)
			}
			if _, _, err := k.UpdateUsers(users("current-user")); err != nil {
				t.Fatal(err)
			}
			if err := connect("retired-user"); err == nil {
				t.Error("removed credentials still authorize new connections")
			}
			if err := connect("current-user"); err != nil {
				t.Errorf("replacement credentials were not published: %v", err)
			}
		})
	}
}

func TestNaiveAuthenticationDuringUserHotSwap(t *testing.T) {
	logger := singlog.NewNOPFactory().NewLogger("test")
	initial := []auth.User{{Username: "allowed", Password: "secret"}}
	inbound, err := naive.NewInbound(context.Background(), nil, logger, "test", option.NaiveInboundOptions{Users: initial, Network: option.NetworkList("tcp")})
	if err != nil {
		t.Fatal(err)
	}
	server := inbound.(*naive.Inbound)
	start := make(chan struct{})
	var work sync.WaitGroup
	work.Add(1)
	go func() {
		defer work.Done()
		<-start
		for range 1000 {
			_ = server.UpdateUsers(initial)
			_ = server.UpdateUsers(nil)
		}
	}()
	close(start)
	for range 1000 {
		request := httptest.NewRequest(http.MethodConnect, "http://example.test:443", nil)
		request.Header.Set("Padding", "test")
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("denied:secret")))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusProxyAuthRequired {
			t.Errorf("unauthorized credentials accepted: %d", response.Code)
		}
	}
	work.Wait()
}
