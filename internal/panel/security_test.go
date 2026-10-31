package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ClaraCora/CPanelde/internal/config"
)

func TestLegacyClientRejectsRedirectBeforeForwardingCredential(t *testing.T) {
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client := NewClient(config.PanelConfig{URL: origin.URL, Token: "test-only-credential"})
	if _, err := client.Handshake(); err == nil {
		t.Fatal("redirect accepted")
	}
	if forwarded.Load() != 0 {
		t.Fatal("credential forwarded to another origin")
	}
}

func TestLegacyClientsRejectRemotePlaintextBeforeDial(t *testing.T) {
	client := NewClient(config.PanelConfig{URL: "http://panel.example", Token: "test-only-credential"})
	if _, err := client.Handshake(); err == nil {
		t.Fatal("plaintext HTTP accepted")
	}
	ws := &WSClient{wsURL: "ws://panel.example/ws", token: "test-only-credential"}
	if err := ws.connect(context.Background()); err == nil {
		t.Fatal("plaintext WebSocket accepted")
	} else if strings.Contains(err.Error(), ws.token) {
		t.Fatal("error exposed credential")
	}
}
