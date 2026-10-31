package v2rayhttp

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"

	"golang.org/x/net/http2"
)

// Track owned connections rather than relying on private http2 pool layouts.
type TrackedHTTP2Transport struct {
	*http2.Transport
	once        sync.Once
	mu          sync.Mutex
	connections map[*trackedConn]struct{}
	generation  uint64
}

func TrackHTTP2Transport(transport *http2.Transport) *TrackedHTTP2Transport {
	return &TrackedHTTP2Transport{Transport: transport, connections: make(map[*trackedConn]struct{})}
}

func (t *TrackedHTTP2Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.once.Do(func() {
		dial := t.DialTLSContext
		t.DialTLSContext = func(ctx context.Context, network, address string, config *tls.Config) (net.Conn, error) {
			t.mu.Lock()
			generation := t.generation
			t.mu.Unlock()
			if dial == nil {
				return nil, net.ErrClosed
			}
			connection, err := dial(ctx, network, address, config)
			if err != nil {
				return nil, err
			}
			tracked := &trackedConn{Conn: connection, owner: t}
			t.mu.Lock()
			if generation != t.generation {
				t.mu.Unlock()
				connection.Close()
				return nil, net.ErrClosed
			}
			t.connections[tracked] = struct{}{}
			t.mu.Unlock()
			if state, ok := connection.(connectionState); ok {
				return &trackedTLSConn{trackedConn: tracked, connectionState: state}, nil
			}
			return tracked, nil
		}
	})
	return t.Transport.RoundTrip(request)
}

func (t *TrackedHTTP2Transport) reset() {
	t.mu.Lock()
	t.generation++
	connections := make([]*trackedConn, 0, len(t.connections))
	for connection := range t.connections {
		connections = append(connections, connection)
	}
	t.mu.Unlock()
	for _, connection := range connections {
		connection.Close()
	}
	t.Transport.CloseIdleConnections()
}

type trackedConn struct {
	net.Conn
	owner *TrackedHTTP2Transport
	once  sync.Once
	err   error
}

func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.owner.mu.Lock()
		delete(c.owner.connections, c)
		c.owner.mu.Unlock()
		c.err = c.Conn.Close()
	})
	return c.err
}

type connectionState interface{ ConnectionState() tls.ConnectionState }
type trackedTLSConn struct {
	*trackedConn
	connectionState
}

func ResetTransport(raw http.RoundTripper) http.RoundTripper {
	switch transport := raw.(type) {
	case *http.Transport:
		transport.CloseIdleConnections()
		return transport.Clone()
	case *TrackedHTTP2Transport:
		transport.reset()
	case interface{ CloseIdleConnections() }:
		transport.CloseIdleConnections()
	}
	return raw
}
