package platformclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/ClaraCora/CPanelde/internal/agentv2"
	"github.com/ClaraCora/CPanelde/internal/nlog"
	"github.com/gorilla/websocket"
)

type Stream struct {
	url            string
	token          string
	machineID      string
	backoffInitial time.Duration
	backoffMax     time.Duration
	onEvent        func(Event)
	onStatus       func(bool)
	connected      atomic.Bool
	v2             *V2Credentials
}

func (s *Stream) SetMachineID(machineID string) *Stream {
	s.machineID = machineID
	return s
}

func (s *Stream) EnableV2(credentials *V2Credentials) error {
	if credentials == nil {
		return errors.New("V2 stream credentials are unavailable")
	}
	if err := validateV2Endpoint(s.url, true); err != nil {
		return err
	}
	s.v2 = credentials
	return nil
}

func NewStream(url, token string, backoffInitial, backoffMax time.Duration, onEvent func(Event), onStatus func(bool)) *Stream {
	if backoffInitial <= 0 {
		backoffInitial = time.Second
	}
	if backoffMax < backoffInitial {
		backoffMax = 60 * time.Second
	}
	return &Stream{
		url: url, token: token, backoffInitial: backoffInitial, backoffMax: backoffMax,
		onEvent: onEvent, onStatus: onStatus,
	}
}

func (s *Stream) IsConnected() bool { return s.connected.Load() }

func (s *Stream) Run(ctx context.Context) {
	backoff := s.backoffInitial
	for ctx.Err() == nil {
		connectedAt, err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			nlog.Core().Warn("device platform stream disconnected", "error", err, "retry_in", backoff)
		}
		if time.Since(connectedAt) > 30*time.Second {
			backoff = s.backoffInitial
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > s.backoffMax {
			backoff = s.backoffMax
		}
	}
}

func (s *Stream) runOnce(ctx context.Context) (time.Time, error) {
	if s.v2 != nil {
		return s.runOnceV2(ctx)
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.token)
	header.Set("X-Corade-Protocol-Version", ProtocolVersion)
	if s.machineID != "" {
		header.Set("X-CPanel-Machine-ID", s.machineID)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, s.url, header)
	if err != nil {
		s.setConnected(false)
		return time.Time{}, err
	}
	connectedAt := time.Now()
	s.setConnected(true)
	conn.SetReadLimit(2 << 20)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer conn.Close()
	defer s.setConnected(false)

	for {
		var event Event
		if err := conn.ReadJSON(&event); err != nil {
			return connectedAt, err
		}
		if event.Type != "" && s.onEvent != nil {
			s.onEvent(event)
		}
	}
}

func (s *Stream) runOnceV2(ctx context.Context) (time.Time, error) {
	publicKey, privateKey, enrollmentKey, enrolled := s.v2.handshakeIdentity()
	handshake, err := agentv2.NewClientHandshake(s.v2.machineID, publicKey, privateKey, enrollmentKey, enrolled, time.Now().UTC())
	if err != nil {
		return time.Time{}, err
	}
	header := http.Header{}
	header.Set("Accept", agentv2.ContentType)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, s.url, header)
	if err != nil {
		s.setConnected(false)
		return time.Time{}, err
	}
	conn.SetReadLimit(2 << 20)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer conn.Close()
	defer s.setConnected(false)

	encoded, err := agentv2.EncodeHandshake(handshake.Request)
	if err != nil {
		return time.Time{}, err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, encoded); err != nil {
		return time.Time{}, err
	}
	messageType, body, err := conn.ReadMessage()
	if err != nil {
		return time.Time{}, err
	}
	if messageType != websocket.BinaryMessage {
		return time.Time{}, errors.New("V2 stream handshake was not binary")
	}
	var response agentv2.HandshakeResponse
	if err := agentv2.DecodeHandshake(body, &response); err != nil {
		return time.Time{}, err
	}
	session, err := handshake.Complete(response, s.v2.panelIdentity)
	if err != nil {
		return time.Time{}, err
	}
	if !response.Enrolled {
		return time.Time{}, errors.New("platform did not accept the V2 stream identity")
	}
	if err := s.v2.markEnrolled(); err != nil {
		return time.Time{}, err
	}
	parsedURL, err := url.Parse(s.url)
	if err != nil {
		return time.Time{}, err
	}
	connectedAt := time.Now()
	s.setConnected(true)

	for {
		messageType, envelope, err := conn.ReadMessage()
		if err != nil {
			return connectedAt, err
		}
		if messageType != websocket.BinaryMessage {
			return connectedAt, errors.New("plaintext V2 stream message was rejected")
		}
		plain, err := session.Open("WS", parsedURL.Path, envelope)
		if err != nil {
			return connectedAt, err
		}
		var wire v2Event
		if err := json.Unmarshal(plain, &wire); err != nil {
			return connectedAt, err
		}
		event := wire.public()
		if event.Type != "" && s.onEvent != nil {
			s.onEvent(event)
		}
	}
}

func (s *Stream) setConnected(value bool) {
	if s.connected.Swap(value) == value {
		return
	}
	if s.onStatus != nil {
		s.onStatus(value)
	}
}
