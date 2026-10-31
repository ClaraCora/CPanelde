package controlsecurity

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestEndpointPolicy(t *testing.T) {
	for _, test := range []struct {
		raw              string
		websocket, valid bool
	}{
		{"https://panel.example/api", false, true}, {"http://127.0.0.1:9000", false, true},
		{"http://[::1]:9000", false, true}, {"http://localhost", false, false},
		{"http://panel.example", false, false}, {"https://user:pass@panel.example", false, false},
		{"https://panel.example#fragment", false, false}, {"ftp://panel.example", false, false},
		{"wss://panel.example/ws", true, true}, {"ws://127.0.0.1/ws", true, true},
		{"ws://panel.example/ws", true, false}, {"https://panel.example/ws", true, false},
	} {
		if valid := ValidateEndpoint(test.raw, test.websocket) == nil; valid != test.valid {
			t.Errorf("%s valid=%v", test.raw, valid)
		}
	}
	if err := RejectRedirect(&http.Request{}, nil); err == nil {
		t.Fatal("redirect allowed")
	}
}

func TestURLAndWrappedErrorsNeverExposeCredentials(t *testing.T) {
	secret := "test-secret+/="
	raw := "https://username:password@panel.example/api?token=" + url.QueryEscape(secret) + "&other=hidden#fragment"
	redacted := RedactedURL(raw)
	for _, forbidden := range []string{"username", "password", "token", "hidden", "fragment"} {
		if strings.Contains(redacted, forbidden) {
			t.Errorf("URL leaked %s", forbidden)
		}
	}
	cause := errors.New("transport failed")
	err := RedactError(&url.Error{Op: "Get", URL: raw, Err: cause}, secret)
	if !errors.Is(err, cause) {
		t.Fatal("error identity lost")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "token=") || strings.Contains(err.Error(), "password") {
		t.Fatal(err)
	}
}
