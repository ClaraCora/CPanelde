package platformclient

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

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
}

func (s *Stream) SetMachineID(machineID string) *Stream {
	s.machineID = machineID
	return s
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

func (s *Stream) setConnected(value bool) {
	if s.connected.Swap(value) == value {
		return
	}
	if s.onStatus != nil {
		s.onStatus(value)
	}
}
