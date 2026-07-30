package agentv2

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

func TestClientHandshakeAndBidirectionalEncryption(t *testing.T) {
	client, server, _, _, _ := pairedSessions(t)

	request, err := client.Seal("POST", "/ca/cc/fwq/jd", []byte(`{"yb":"7"}`))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := server.Open("POST", "/ca/cc/fwq/jd", request)
	if err != nil || string(plain) != `{"yb":"7"}` {
		t.Fatalf("server open = %q, %v", plain, err)
	}

	response, err := server.Seal("POST", "/ca/cc/fwq/jd", []byte(`{"jd":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	plain, err = client.Open("POST", "/ca/cc/fwq/jd", response)
	if err != nil || string(plain) != `{"jd":[]}` {
		t.Fatalf("client open = %q, %v", plain, err)
	}
}

func TestEncryptedEnvelopeBindsMethodPathAndRejectsReplay(t *testing.T) {
	client, server, _, _, _ := pairedSessions(t)
	envelope, err := client.Seal("POST", "/ca/cc/jd/12/pz", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Open("GET", "/ca/cc/jd/12/pz", envelope); err == nil {
		t.Fatal("method substitution was accepted")
	}
	if _, err := server.Open("POST", "/ca/cc/jd/13/pz", envelope); err == nil {
		t.Fatal("path substitution was accepted")
	}
	if _, err := server.Open("POST", "/ca/cc/jd/12/pz", envelope); err != nil {
		t.Fatalf("valid envelope rejected after forged attempts: %v", err)
	}
	if _, err := server.Open("POST", "/ca/cc/jd/12/pz", envelope); err != ErrReplay {
		t.Fatalf("replay error = %v, want %v", err, ErrReplay)
	}
}

func TestForgedHighSequenceDoesNotPoisonReplayWindow(t *testing.T) {
	client, server, _, _, _ := pairedSessions(t)
	valid, err := client.Seal("POST", "/ca/cc/yc", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	forged := append([]byte(nil), valid...)
	for i := 1 + sessionIDSize; i < headerSize; i++ {
		forged[i] = 0xff
	}
	if _, err := server.Open("POST", "/ca/cc/yc", forged); err == nil {
		t.Fatal("forged high-sequence envelope was accepted")
	}
	if _, err := server.Open("POST", "/ca/cc/yc", valid); err != nil {
		t.Fatalf("valid envelope rejected after forgery: %v", err)
	}
}

func TestHandshakePinsPanelIdentity(t *testing.T) {
	_, _, handshake, response, _ := pairedSessions(t)
	wrongPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handshake.Complete(response, wrongPublic); err == nil {
		t.Fatal("handshake accepted a different panel identity")
	}
}

func pairedSessions(t *testing.T) (*Session, *Session, *ClientHandshake, HandshakeResponse, ed25519.PublicKey) {
	t.Helper()
	agentPublic, agentPrivate, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	enrollmentKey := []byte("0123456789abcdef0123456789abcdef")
	handshake, err := NewClientHandshake("mch_test", agentPublic, agentPrivate, enrollmentKey, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	proof, err := base64.RawURLEncoding.DecodeString(handshake.Request.Proof)
	if err != nil || !hmac.Equal(proof, hmacSHA256(enrollmentKey, RequestTranscript(handshake.Request))) {
		t.Fatal("enrollment proof is invalid")
	}

	panelPublic, panelPrivate, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	serverEphemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientEphemeralBytes, err := base64.RawURLEncoding.DecodeString(handshake.Request.EphemeralPublic)
	if err != nil {
		t.Fatal(err)
	}
	clientEphemeral, err := ecdh.X25519().NewPublicKey(clientEphemeralBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := serverEphemeral.ECDH(clientEphemeral)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := make([]byte, sessionIDSize)
	serverNonce := make([]byte, 32)
	if _, err := rand.Read(sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(serverNonce); err != nil {
		t.Fatal(err)
	}
	response := HandshakeResponse{
		Version: ProtocolVersion, SessionID: base64.RawURLEncoding.EncodeToString(sessionID),
		IdentityPublic:  base64.RawURLEncoding.EncodeToString(panelPublic),
		EphemeralPublic: base64.RawURLEncoding.EncodeToString(serverEphemeral.PublicKey().Bytes()),
		Nonce:           base64.RawURLEncoding.EncodeToString(serverNonce), Timestamp: time.Now().UTC().Unix(), Enrolled: true,
	}
	requestTranscript := RequestTranscript(handshake.Request)
	responseTranscript := ResponseTranscript(requestTranscript, response)
	response.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(panelPrivate, responseTranscript))
	serverSession, err := deriveSession(shared, transcriptHash(requestTranscript, responseTranscript), sessionID, handshake.Request.MachineID, true)
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := handshake.Complete(response, panelPublic)
	if err != nil {
		t.Fatal(err)
	}
	return clientSession, serverSession, handshake, response, panelPublic
}
