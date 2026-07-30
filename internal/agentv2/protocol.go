package agentv2

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/hkdf"
)

const (
	ProtocolVersion = "2"
	ContentType     = "application/octet-stream"
	EnvelopeVersion = byte(0x02)
	ModeEnroll      = "dj"
	ModeConnect     = "lj"
	sessionIDSize   = 16
	nonceSize       = 12
	headerSize      = 1 + sessionIDSize + 8
	tagSize         = 16
)

var (
	ErrInvalidEnvelope = errors.New("invalid encrypted envelope")
	ErrReplay          = errors.New("replayed or stale encrypted envelope")
	ErrWrongSession    = errors.New("encrypted envelope belongs to another session")
)

type HandshakeRequest struct {
	Version         string `json:"xybb"`
	Mode            string `json:"lx"`
	MachineID       string `json:"fwqbh"`
	IdentityPublic  string `json:"sfgy"`
	EphemeralPublic string `json:"lsgy"`
	Nonce           string `json:"sjm"`
	Timestamp       int64  `json:"sjc"`
	Proof           string `json:"zm"`
}

type HandshakeResponse struct {
	Version         string `json:"xybb"`
	SessionID       string `json:"hhbh"`
	IdentityPublic  string `json:"sfgy"`
	EphemeralPublic string `json:"lsgy"`
	Nonce           string `json:"sjm"`
	Timestamp       int64  `json:"sjc"`
	Enrolled        bool   `json:"ydj"`
	Signature       string `json:"qm"`
}

type ClientHandshake struct {
	Request           HandshakeRequest
	ephemeralPrivate  *ecdh.PrivateKey
	requestTranscript []byte
}

func GenerateIdentity() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func ParsePublicKey(value string) (ed25519.PublicKey, error) {
	decoded, err := decodeSized(value, ed25519.PublicKeySize)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(decoded), nil
}

func EncodePrivateSeed(key ed25519.PrivateKey) string { return encode(key.Seed()) }

func DecodePrivateSeed(value string) (ed25519.PrivateKey, error) {
	seed, err := decodeSized(value, ed25519.SeedSize)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func NewClientHandshake(machineID string, identityPublic ed25519.PublicKey, identityPrivate ed25519.PrivateKey, enrollmentKey []byte, enrolled bool, now time.Time) (*ClientHandshake, error) {
	if len(identityPublic) != ed25519.PublicKeySize || len(identityPrivate) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid Agent identity key")
	}
	ephemeralPrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate Agent ephemeral key: %w", err)
	}
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	request := HandshakeRequest{Version: ProtocolVersion, Mode: ModeEnroll, MachineID: machineID, IdentityPublic: encode(identityPublic), EphemeralPublic: encode(ephemeralPrivate.PublicKey().Bytes()), Nonce: encode(nonce), Timestamp: now.UTC().Unix()}
	if enrolled {
		request.Mode = ModeConnect
	}
	transcript := RequestTranscript(request)
	if enrolled {
		request.Proof = encode(ed25519.Sign(identityPrivate, transcript))
	} else {
		if len(enrollmentKey) < 32 {
			return nil, errors.New("enrollment key must contain at least 32 bytes")
		}
		request.Proof = encode(hmacSHA256(enrollmentKey, transcript))
	}
	return &ClientHandshake{Request: request, ephemeralPrivate: ephemeralPrivate, requestTranscript: transcript}, nil
}

func (h *ClientHandshake) Complete(response HandshakeResponse, expectedPanelIdentity ed25519.PublicKey) (*Session, error) {
	if h == nil || h.ephemeralPrivate == nil {
		return nil, errors.New("client handshake is not initialized")
	}
	if response.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported response protocol %q", response.Version)
	}
	panelIdentity, err := ParsePublicKey(response.IdentityPublic)
	if err != nil || !hmac.Equal(panelIdentity, expectedPanelIdentity) {
		return nil, errors.New("panel identity does not match pinned public key")
	}
	serverEphemeral, err := decodeSized(response.EphemeralPublic, 32)
	if err != nil {
		return nil, err
	}
	sessionID, err := decodeSized(response.SessionID, sessionIDSize)
	if err != nil {
		return nil, err
	}
	if _, err := decodeSized(response.Nonce, 32); err != nil {
		return nil, err
	}
	signature, err := decodeSized(response.Signature, ed25519.SignatureSize)
	if err != nil {
		return nil, err
	}
	responseTranscript := ResponseTranscript(h.requestTranscript, response)
	if !ed25519.Verify(expectedPanelIdentity, responseTranscript, signature) {
		return nil, errors.New("panel handshake signature is invalid")
	}
	peer, err := ecdh.X25519().NewPublicKey(serverEphemeral)
	if err != nil {
		return nil, err
	}
	shared, err := h.ephemeralPrivate.ECDH(peer)
	if err != nil {
		return nil, err
	}
	return deriveSession(shared, transcriptHash(h.requestTranscript, responseTranscript), sessionID, h.Request.MachineID, false)
}

func RequestTranscript(request HandshakeRequest) []byte {
	var out []byte
	for _, field := range []string{"ca2", request.Version, request.Mode, request.MachineID, request.IdentityPublic, request.EphemeralPublic, request.Nonce} {
		out = appendField(out, field)
	}
	return appendUint64(out, uint64(request.Timestamp))
}

func ResponseTranscript(request []byte, response HandshakeResponse) []byte {
	out := append([]byte(nil), request...)
	for _, field := range []string{response.Version, response.SessionID, response.IdentityPublic, response.EphemeralPublic, response.Nonce} {
		out = appendField(out, field)
	}
	out = appendUint64(out, uint64(response.Timestamp))
	if response.Enrolled {
		out = append(out, 1)
	} else {
		out = append(out, 0)
	}
	return out
}

func EncodeHandshake(value any) ([]byte, error) { return json.Marshal(value) }

func DecodeHandshake(data []byte, target any) error {
	if len(data) == 0 || len(data) > 16<<10 {
		return errors.New("invalid handshake size")
	}
	return json.Unmarshal(data, target)
}

type Session struct {
	machineID     string
	sessionID     [sessionIDSize]byte
	sendAEAD      cipher.AEAD
	recvAEAD      cipher.AEAD
	sendNonce     [nonceSize]byte
	recvNonce     [nonceSize]byte
	sendDirection byte
	recvDirection byte
	sendSeq       atomic.Uint64
	recvMu        sync.Mutex
	recvWindow    replayWindow
}

func (s *Session) ID() string { return encode(s.sessionID[:]) }

func (s *Session) Seal(method, path string, plaintext []byte) ([]byte, error) {
	seq := s.sendSeq.Add(1)
	if seq == 0 {
		return nil, errors.New("encrypted session sequence exhausted")
	}
	nonce := makeNonce(s.sendNonce, seq)
	aad := associatedData(s.sessionID, s.machineID, method, path, s.sendDirection, seq)
	ciphertext := s.sendAEAD.Seal(nil, nonce[:], plaintext, aad)
	out := make([]byte, headerSize+len(ciphertext))
	out[0] = EnvelopeVersion
	copy(out[1:1+sessionIDSize], s.sessionID[:])
	binary.BigEndian.PutUint64(out[1+sessionIDSize:headerSize], seq)
	copy(out[headerSize:], ciphertext)
	return out, nil
}

func (s *Session) Open(method, path string, envelope []byte) ([]byte, error) {
	if len(envelope) < headerSize+tagSize || envelope[0] != EnvelopeVersion {
		return nil, ErrInvalidEnvelope
	}
	if !hmac.Equal(envelope[1:1+sessionIDSize], s.sessionID[:]) {
		return nil, ErrWrongSession
	}
	seq := binary.BigEndian.Uint64(envelope[1+sessionIDSize : headerSize])
	if seq == 0 {
		return nil, ErrInvalidEnvelope
	}
	s.recvMu.Lock()
	defer s.recvMu.Unlock()
	candidate := s.recvWindow
	if !candidate.accept(seq) {
		return nil, ErrReplay
	}
	nonce := makeNonce(s.recvNonce, seq)
	aad := associatedData(s.sessionID, s.machineID, method, path, s.recvDirection, seq)
	plain, err := s.recvAEAD.Open(nil, nonce[:], envelope[headerSize:], aad)
	if err != nil {
		return nil, ErrInvalidEnvelope
	}
	s.recvWindow = candidate
	return plain, nil
}

func SessionIDFromEnvelope(envelope []byte) (string, error) {
	if len(envelope) < headerSize+tagSize || envelope[0] != EnvelopeVersion {
		return "", ErrInvalidEnvelope
	}
	return encode(envelope[1 : 1+sessionIDSize]), nil
}

type replayWindow struct {
	maxSeq uint64
	bitmap uint64
}

func (w *replayWindow) accept(seq uint64) bool {
	if seq == 0 {
		return false
	}
	if seq > w.maxSeq {
		shift := seq - w.maxSeq
		if shift >= 64 {
			w.bitmap = 0
		} else {
			w.bitmap <<= shift
		}
		w.maxSeq = seq
		w.bitmap |= 1
		return true
	}
	difference := w.maxSeq - seq
	if difference >= 64 {
		return false
	}
	bit := uint64(1) << difference
	if w.bitmap&bit != 0 {
		return false
	}
	w.bitmap |= bit
	return true
}

func deriveSession(shared, salt, sessionID []byte, machineID string, server bool) (*Session, error) {
	if len(sessionID) != sessionIDSize || machineID == "" {
		return nil, errors.New("invalid encrypted session binding")
	}
	reader := hkdf.New(sha256.New, shared, salt, []byte("ca2"))
	material := make([]byte, 32+32+nonceSize+nonceSize)
	if _, err := io.ReadFull(reader, material); err != nil {
		return nil, err
	}
	newAEAD := func(key []byte) (cipher.AEAD, error) {
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	}
	c2s, err := newAEAD(material[:32])
	if err != nil {
		return nil, err
	}
	s2c, err := newAEAD(material[32:64])
	if err != nil {
		return nil, err
	}
	session := &Session{machineID: machineID}
	copy(session.sessionID[:], sessionID)
	if server {
		session.sendAEAD, session.recvAEAD = s2c, c2s
		copy(session.sendNonce[:], material[64+nonceSize:])
		copy(session.recvNonce[:], material[64:64+nonceSize])
		session.sendDirection, session.recvDirection = 1, 0
	} else {
		session.sendAEAD, session.recvAEAD = c2s, s2c
		copy(session.sendNonce[:], material[64:64+nonceSize])
		copy(session.recvNonce[:], material[64+nonceSize:])
		session.sendDirection, session.recvDirection = 0, 1
	}
	return session, nil
}

func associatedData(sessionID [sessionIDSize]byte, machineID, method, path string, direction byte, seq uint64) []byte {
	out := []byte{EnvelopeVersion, direction}
	out = append(out, sessionID[:]...)
	for _, field := range []string{machineID, method, path} {
		out = appendField(out, field)
	}
	return appendUint64(out, seq)
}

func transcriptHash(request, response []byte) []byte {
	hash := sha256.New()
	hash.Write(request)
	hash.Write(response)
	return hash.Sum(nil)
}
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
func appendField(out []byte, value string) []byte {
	out = appendUint64(out, uint64(len(value)))
	return append(out, value...)
}
func appendUint64(out []byte, value uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	return append(out, b[:]...)
}
func makeNonce(base [nonceSize]byte, seq uint64) [nonceSize]byte {
	nonce := base
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], seq)
	for i := range b {
		nonce[nonceSize-len(b)+i] ^= b[i]
	}
	return nonce
}
func encode(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
func decodeSized(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != size {
		return nil, errors.New("invalid encoded value")
	}
	return decoded, nil
}
