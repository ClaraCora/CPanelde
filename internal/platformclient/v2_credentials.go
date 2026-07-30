package platformclient

import (
	"crypto/ed25519"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ClaraCora/CPanelde/internal/agentv2"
)

type v2IdentityFile struct {
	Version        int    `json:"bb"`
	Seed           string `json:"zz"`
	PanelPublicKey string `json:"mbgy"`
	Enrolled       bool   `json:"ydj"`
}

// V2Credentials are shared by the HTTP and WebSocket clients so both use the
// same pinned panel identity and per-machine Agent identity.
type V2Credentials struct {
	machineID     string
	enrollmentKey []byte
	panelIdentity ed25519.PublicKey
	identityPath  string

	mu              sync.Mutex
	identityPublic  ed25519.PublicKey
	identityPrivate ed25519.PrivateKey
	enrolled        bool
}

func loadV2Credentials(machineID, enrollmentKey, panelPublicKey, identityPath string) (*V2Credentials, error) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return nil, errors.New("V2 machine ID is required")
	}
	if len(enrollmentKey) < 32 {
		return nil, errors.New("V2 enrollment key must contain at least 32 bytes")
	}
	panelIdentity, err := agentv2.ParsePublicKey(strings.TrimSpace(panelPublicKey))
	if err != nil {
		return nil, fmt.Errorf("parse pinned panel identity: %w", err)
	}
	identityPath = strings.TrimSpace(identityPath)
	if identityPath == "" {
		return nil, errors.New("V2 Agent identity path is required")
	}

	credentials := &V2Credentials{
		machineID: machineID, enrollmentKey: []byte(enrollmentKey), panelIdentity: panelIdentity,
		identityPath: identityPath,
	}
	if err := credentials.loadOrCreateIdentity(); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (c *V2Credentials) loadOrCreateIdentity() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.identityPath)
	if err == nil {
		var stored v2IdentityFile
		if json.Unmarshal(data, &stored) != nil || stored.Version != 2 {
			return errors.New("stored V2 Agent identity is invalid")
		}
		privateKey, decodeErr := agentv2.DecodePrivateSeed(stored.Seed)
		if decodeErr != nil {
			return fmt.Errorf("decode stored V2 Agent identity: %w", decodeErr)
		}
		c.identityPrivate = privateKey
		c.identityPublic = privateKey.Public().(ed25519.PublicKey)
		c.enrolled = stored.Enrolled
		if stored.PanelPublicKey != "" {
			storedPanelIdentity, panelErr := agentv2.ParsePublicKey(stored.PanelPublicKey)
			if panelErr != nil || !hmac.Equal(storedPanelIdentity, c.panelIdentity) {
				return errors.New("stored V2 panel identity does not match the configured public key")
			}
		} else if err := c.writeIdentityLocked(); err != nil {
			return err
		}
		if chmodErr := os.Chmod(c.identityPath, 0o600); chmodErr != nil {
			return fmt.Errorf("protect V2 Agent identity: %w", chmodErr)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("read V2 Agent identity: %w", err)
	}

	publicKey, privateKey, err := agentv2.GenerateIdentity()
	if err != nil {
		return fmt.Errorf("generate V2 Agent identity: %w", err)
	}
	c.identityPublic, c.identityPrivate, c.enrolled = publicKey, privateKey, false
	return c.writeIdentityLocked()
}

func (c *V2Credentials) handshakeIdentity() (ed25519.PublicKey, ed25519.PrivateKey, []byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append(ed25519.PublicKey(nil), c.identityPublic...), append(ed25519.PrivateKey(nil), c.identityPrivate...), append([]byte(nil), c.enrollmentKey...), c.enrolled
}

func (c *V2Credentials) markEnrolled() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.enrolled {
		return nil
	}
	c.enrolled = true
	if err := c.writeIdentityLocked(); err != nil {
		c.enrolled = false
		return err
	}
	return nil
}

func (c *V2Credentials) markNeedsEnrollment() {
	c.mu.Lock()
	c.enrolled = false
	c.mu.Unlock()
}

func (c *V2Credentials) writeIdentityLocked() error {
	if err := os.MkdirAll(filepath.Dir(c.identityPath), 0o700); err != nil {
		return fmt.Errorf("create V2 Agent identity directory: %w", err)
	}
	encoded, err := json.Marshal(v2IdentityFile{
		Version: 2, Seed: agentv2.EncodePrivateSeed(c.identityPrivate),
		PanelPublicKey: base64.RawURLEncoding.EncodeToString(c.panelIdentity), Enrolled: c.enrolled,
	})
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(c.identityPath), ".agent-v2-*")
	if err != nil {
		return fmt.Errorf("create V2 Agent identity file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, c.identityPath); err != nil {
		return fmt.Errorf("persist V2 Agent identity: %w", err)
	}
	return os.Chmod(c.identityPath, 0o600)
}

func StoredV2PanelPublicKey(identityPath string) (string, error) {
	data, err := os.ReadFile(strings.TrimSpace(identityPath))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read stored V2 identity: %w", err)
	}
	var stored v2IdentityFile
	if json.Unmarshal(data, &stored) != nil || stored.Version != 2 || stored.PanelPublicKey == "" {
		return "", errors.New("stored V2 panel identity is invalid")
	}
	if _, err := agentv2.ParsePublicKey(stored.PanelPublicKey); err != nil {
		return "", errors.New("stored V2 panel identity is invalid")
	}
	return stored.PanelPublicKey, nil
}

func replaceFile(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	// Windows does not replace an existing destination with os.Rename. The
	// Agent runs on Linux in production, but keep local development reliable.
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, target)
}

func validateV2Endpoint(rawURL string, websocket bool) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return errors.New("V2 endpoint URL is invalid")
	}
	secureScheme, localScheme := "https", "http"
	if websocket {
		secureScheme, localScheme = "wss", "ws"
	}
	if strings.EqualFold(parsed.Scheme, secureScheme) {
		return nil
	}
	if !strings.EqualFold(parsed.Scheme, localScheme) || !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("V2 endpoint must use %s outside a loopback address", secureScheme)
	}
	return nil
}

func ValidateControlEndpoint(rawURL string) error {
	return validateV2Endpoint(rawURL, false)
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}
