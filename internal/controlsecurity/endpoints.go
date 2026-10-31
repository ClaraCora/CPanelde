// Package controlsecurity applies one transport policy to every control mode.
package controlsecurity

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func ValidateEndpoint(raw string, websocket bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return errors.New("control endpoint URL is invalid or contains userinfo/fragment")
	}
	secure, local := "https", "http"
	if websocket {
		secure, local = "wss", "ws"
	}
	if strings.EqualFold(u.Scheme, secure) {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if !strings.EqualFold(u.Scheme, local) || ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("control endpoint must use %s outside a numeric loopback address", secure)
	}
	return nil
}

// Control APIs never need redirects. Refusing them also prevents forwarding a
// credential-bearing body or query to a different host or a plaintext URL.
func RejectRedirect(*http.Request, []*http.Request) error {
	return errors.New("control API redirects are disabled")
}

func RedactedURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid URL]"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	u.ForceQuery = false
	return u.String()
}

type safeError struct {
	message string
	cause   error
}

func (e *safeError) Error() string { return e.message }
func (e *safeError) Unwrap() error { return e.cause }

func RedactError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	var endpointError *url.Error
	if errors.As(err, &endpointError) {
		message = fmt.Sprintf("%s %s: %s", endpointError.Op, RedactedURL(endpointError.URL), RedactError(endpointError.Err, secrets...))
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
			message = strings.ReplaceAll(message, url.QueryEscape(secret), "[redacted]")
		}
	}
	return &safeError{message: message, cause: err}
}
