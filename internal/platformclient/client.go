package platformclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type APIError struct {
	Status  int
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("platform request failed with status %d", e.Status)
	}
	return fmt.Sprintf("platform request failed: %s: %s", e.Code, e.Message)
}

type Client struct {
	baseURL    string
	token      string
	machineID  string
	httpClient *http.Client
	success    atomic.Uint64
	failure    atomic.Uint64
}

func New(baseURL, token string, machineID ...string) *Client {
	configuredMachineID := ""
	if len(machineID) > 0 {
		configuredMachineID = strings.TrimSpace(machineID[0])
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		machineID: configuredMachineID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *Client) Handshake(ctx context.Context) (Handshake, error) {
	var result Handshake
	err := c.request(ctx, http.MethodPost, "/ca/cc/ws", map[string]string{"protocol_version": ProtocolVersion}, nil, &result)
	if err == nil && result.ProtocolVersion != ProtocolVersion {
		return Handshake{}, fmt.Errorf("unsupported platform protocol version %q", result.ProtocolVersion)
	}
	return result, err
}

func (c *Client) Nodes(ctx context.Context) (NodesResponse, error) {
	var result NodesResponse
	err := c.request(ctx, http.MethodGet, "/ca/cc/fwq/jd", nil, nil, &result)
	return result, err
}

func (c *Client) NodeSpec(ctx context.Context, nodeID int) (NodeSpec, error) {
	var result NodeSpec
	err := c.request(ctx, http.MethodGet, "/ca/cc/jd/"+strconv.Itoa(nodeID)+"/pz", nil, nil, &result)
	return result, err
}

func (c *Client) NodeUsers(ctx context.Context, nodeID int) (UsersResponse, error) {
	var result UsersResponse
	err := c.request(ctx, http.MethodGet, "/ca/cc/jd/"+strconv.Itoa(nodeID)+"/yh", nil, nil, &result)
	return result, err
}

func (c *Client) Changes(ctx context.Context, cursor string) (ChangesResponse, error) {
	path := "/ca/cc/bg"
	if cursor != "" {
		path += "?" + url.Values{"yb": []string{cursor}}.Encode()
	}
	var result ChangesResponse
	err := c.request(ctx, http.MethodGet, path, nil, nil, &result)
	return result, err
}

func (c *Client) SendHeartbeat(ctx context.Context, heartbeat Heartbeat) error {
	return c.request(ctx, http.MethodPost, "/ca/cc/fwq/xt", heartbeat, nil, nil)
}

func (c *Client) SendTelemetry(ctx context.Context, key string, batch TelemetryBatch) error {
	if key == "" {
		key = NewIdempotencyKey("tel")
	}
	return c.request(ctx, http.MethodPost, "/ca/cc/yc", batch, map[string]string{"Idempotency-Key": key}, nil)
}

func (c *Client) Metrics() (success, failure uint64) {
	return c.success.Load(), c.failure.Load()
}

func (c *Client) Token() string { return c.token }

func NewIdempotencyKey(prefix string) string {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(random)
}

func (c *Client) request(ctx context.Context, method, path string, payload any, headers map[string]string, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode platform request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Corade-Protocol-Version", ProtocolVersion)
	if c.machineID != "" {
		req.Header.Set("X-CPanel-Machine-ID", c.machineID)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.failure.Add(1)
		return err
	}
	defer drainAndClose(resp.Body)

	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *APIError       `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(&envelope); err != nil {
		c.failure.Add(1)
		return fmt.Errorf("decode platform response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Error != nil {
		c.failure.Add(1)
		if envelope.Error == nil {
			envelope.Error = &APIError{}
		}
		envelope.Error.Status = resp.StatusCode
		return envelope.Error
	}
	if target != nil {
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			c.failure.Add(1)
			return fmt.Errorf("decode platform data: %w", err)
		}
	}
	c.success.Add(1)
	return nil
}

func drainAndClose(body io.ReadCloser) {
	_, _ = io.CopyN(io.Discard, body, 512)
	_ = body.Close()
}
