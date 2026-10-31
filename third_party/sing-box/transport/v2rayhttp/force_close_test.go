package v2rayhttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestTrackedTransportClosesActiveTLSAndReconnects(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			w.Write([]byte("ready\n"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Write([]byte("reconnected"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	transport := TrackHTTP2Transport(&http2.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}, DialTLSContext: func(ctx context.Context, network, address string, cfg *tls.Config) (net.Conn, error) {
		return (&tls.Dialer{NetDialer: &net.Dialer{Timeout: time.Second}, Config: cfg}).DialContext(ctx, network, address)
	}})
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get(server.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	if response.ProtoMajor != 2 {
		t.Fatal("request did not use HTTP/2")
	}
	if _, err := io.ReadFull(response.Body, make([]byte, 6)); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	tracked := len(transport.connections)
	transport.mu.Unlock()
	if tracked != 1 {
		t.Fatalf("tracked %d connections", tracked)
	}
	ResetTransport(transport)
	if _, err := response.Body.Read(make([]byte, 1)); err == nil {
		t.Fatal("active connection survived reset")
	}
	response.Body.Close()
	response, err = client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(data) != "reconnected" {
		t.Fatalf("reconnect: %q %v", data, err)
	}
	ResetTransport(transport)
	transport.mu.Lock()
	tracked = len(transport.connections)
	transport.mu.Unlock()
	if tracked != 0 {
		t.Fatal("closed connections retained")
	}
}

func TestTrackedTransportClosesConnectionWhoseDialRacesReset(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	left, right := net.Pipe()
	defer right.Close()
	transport := TrackHTTP2Transport(&http2.Transport{DialTLSContext: func(context.Context, string, string, *tls.Config) (net.Conn, error) {
		close(started)
		<-release
		return left, nil
	}})
	result := make(chan error, 1)
	go func() {
		request, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
		_, err := transport.RoundTrip(request)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dial not started")
	}
	ResetTransport(transport)
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("racing dial accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("racing dial did not terminate")
	}
	right.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := right.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("racing connection not closed: %v", err)
	}
}
