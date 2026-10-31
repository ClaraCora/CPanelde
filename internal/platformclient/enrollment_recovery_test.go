package platformclient

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClaraCora/CPanelde/internal/agentv2"
)

func TestV2EnrollmentRecoveryUsesIdentityBeforeOneUseCredential(t *testing.T) {
	for _, reset := range []bool{false, true} {
		name := "enrollment-response-was-lost"
		if reset {
			name = "administrator-reset-with-existing-identity-file"
		}
		t.Run(name, func(t *testing.T) {
			panelPublic, panelPrivate, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			secret := make([]byte, 32)
			if _, err := rand.Read(secret); err != nil {
				t.Fatal(err)
			}
			token := base64.RawURLEncoding.EncodeToString(secret)
			pin := base64.RawURLEncoding.EncodeToString(panelPublic)
			path := filepath.Join(t.TempDir(), "identity.json")
			original, err := loadV2Credentials("mch_recovery", token, pin, path)
			if err != nil {
				t.Fatal(err)
			}
			if reset {
				if err := original.markEnrolled(); err != nil {
					t.Fatal(err)
				}
			}
			credentials, err := loadV2Credentials("mch_recovery", token, pin, path)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				var request agentv2.HandshakeRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				proof, err := base64.RawURLEncoding.DecodeString(request.Proof)
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if call == 1 {
					public, _, _, _ := credentials.handshakeIdentity()
					if request.Mode != agentv2.ModeConnect || !ed25519.Verify(public, agentv2.RequestTranscript(request), proof) {
						t.Error("first request did not prove the existing private identity")
						w.WriteHeader(401)
						return
					}
					if reset {
						w.WriteHeader(401)
						return
					}
				} else {
					mac := hmac.New(sha256.New, []byte(token))
					mac.Write(agentv2.RequestTranscript(request))
					if !reset || call != 2 || request.Mode != agentv2.ModeEnroll || !hmac.Equal(mac.Sum(nil), proof) {
						t.Error("replacement credential was lost or enrollment was retried incorrectly")
						w.WriteHeader(401)
						return
					}
				}
				ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				session, nonce := make([]byte, 16), make([]byte, 32)
				rand.Read(session)
				rand.Read(nonce)
				encode := base64.RawURLEncoding.EncodeToString
				response := agentv2.HandshakeResponse{Version: agentv2.ProtocolVersion, SessionID: encode(session), IdentityPublic: pin, EphemeralPublic: encode(ephemeral.PublicKey().Bytes()), Nonce: encode(nonce), Timestamp: time.Now().Unix(), Enrolled: true}
				response.Signature = encode(ed25519.Sign(panelPrivate, agentv2.ResponseTranscript(agentv2.RequestTranscript(request), response)))
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			if _, err := establishV2Session(context.Background(), server.Client(), server.URL, credentials, false); err != nil {
				t.Fatal(err)
			}
			want := int32(1)
			if reset {
				want = 2
			}
			if calls.Load() != want {
				t.Fatalf("requests=%d, want=%d", calls.Load(), want)
			}
			if _, _, key, enrolled := credentials.handshakeIdentity(); len(key) != 0 || !enrolled {
				t.Fatal("successful enrollment retained the credential or lost enrollment state")
			}
			if _, err := loadV2Credentials("mch_recovery", "", pin, path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
