package platformclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestV2IdentityPersistsAcrossRestarts(t *testing.T) {
	panelPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agent-v2-identity.json")
	panelKey := base64.RawURLEncoding.EncodeToString(panelPublic)
	enrollmentKey := "0123456789abcdef0123456789abcdef"
	first, err := loadV2Credentials("mch_test", enrollmentKey, panelKey, path)
	if err != nil {
		t.Fatal(err)
	}
	firstPublic, _, _, enrolled := first.handshakeIdentity()
	if enrolled {
		t.Fatal("new identity must not start enrolled")
	}
	if err := first.markEnrolled(); err != nil {
		t.Fatal(err)
	}
	second, err := loadV2Credentials("mch_test", enrollmentKey, panelKey, path)
	if err != nil {
		t.Fatal(err)
	}
	secondPublic, _, _, enrolled := second.handshakeIdentity()
	if !enrolled || !firstPublic.Equal(secondPublic) {
		t.Fatal("persisted identity or enrollment status changed")
	}
	storedPanelKey, err := StoredV2PanelPublicKey(path)
	if err != nil || storedPanelKey != panelKey {
		t.Fatalf("stored panel key = %q, %v", storedPanelKey, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("identity permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestV2EndpointRequiresTLSOutsideLoopback(t *testing.T) {
	for _, test := range []struct {
		url       string
		websocket bool
		valid     bool
	}{
		{url: "https://panel.example.com", valid: true},
		{url: "http://127.0.0.1:8256", valid: true},
		{url: "http://[::1]:8256", valid: true},
		{url: "http://panel.example.com", valid: false},
		{url: "wss://panel.example.com/ca/cc/td", websocket: true, valid: true},
		{url: "ws://127.0.0.1:8256/ca/cc/td", websocket: true, valid: true},
		{url: "ws://panel.example.com/ca/cc/td", websocket: true, valid: false},
	} {
		err := validateV2Endpoint(test.url, test.websocket)
		if (err == nil) != test.valid {
			t.Errorf("validateV2Endpoint(%q, %v) = %v", test.url, test.websocket, err)
		}
	}
}
