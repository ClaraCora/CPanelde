package controlplane

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/nlog"
	"github.com/ClaraCora/CPanelde/internal/platformclient"
)

type DevicePlatformControlPlane struct {
	client       *platformclient.Client
	nodeID       int
	kcfg         config.KernelConfig
	pushInterval int
	pullInterval int
	push         PushClient
	registerFn   func(statuses chan<- StatusChange) *NodeMailbox
	revision     atomic.Int64
}

func NewDevicePlatformControlPlane(
	client *platformclient.Client,
	nodeID int,
	kcfg config.KernelConfig,
	pushInterval, pullInterval int,
	push PushClient,
	registerFn func(statuses chan<- StatusChange) *NodeMailbox,
) *DevicePlatformControlPlane {
	return &DevicePlatformControlPlane{
		client: client, nodeID: nodeID, kcfg: kcfg, pushInterval: pushInterval, pullInterval: pullInterval,
		push: push, registerFn: registerFn,
	}
}

func (p *DevicePlatformControlPlane) SupportsPolling() bool       { return true }
func (p *DevicePlatformControlPlane) SupportsDiscovery() bool     { return false }
func (p *DevicePlatformControlPlane) SupportsReporting() bool     { return true }
func (p *DevicePlatformControlPlane) SupportsDeviceReports() bool { return true }

func (p *DevicePlatformControlPlane) Initial(
	ctx context.Context,
	metricsFn func() map[string]interface{},
	events chan<- Event,
	statuses chan<- StatusChange,
) (Bootstrap, error) {
	snapshot, err := p.snapshot(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	bootstrap := Bootstrap{
		PushInterval: p.pushInterval,
		PullInterval: p.pullInterval,
		Push:         p.push,
		Config:       snapshot.Config,
		Users:        snapshot.Users,
	}
	if p.registerFn != nil {
		bootstrap.Mailbox = p.registerFn(statuses)
	}
	return bootstrap, nil
}

func (p *DevicePlatformControlPlane) Poll(ctx context.Context) (Snapshot, error) {
	return p.snapshot(ctx)
}

func (p *DevicePlatformControlPlane) snapshot(ctx context.Context) (Snapshot, error) {
	spec, err := p.client.NodeSpec(ctx, p.nodeID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("device platform node spec: %w", err)
	}
	if spec.NodeID != p.nodeID {
		return Snapshot{}, fmt.Errorf("device platform returned node %d for requested node %d", spec.NodeID, p.nodeID)
	}
	p.revision.Store(int64(spec.Revision))
	modelSpec, err := spec.Model(p.kcfg)
	if err != nil {
		return Snapshot{}, fmt.Errorf("device platform normalize node spec: %w", err)
	}
	users, err := p.client.NodeUsers(ctx, p.nodeID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("device platform node users: %w", err)
	}
	return Snapshot{Config: modelSpec, Users: users.Models()}, nil
}

func (p *DevicePlatformControlPlane) Discover(
	ctx context.Context,
	metricsFn func() map[string]interface{},
	events chan<- Event,
	statuses chan<- StatusChange,
) (PushClient, error) {
	return nil, nil
}

func (p *DevicePlatformControlPlane) Report(payload ReportPayload) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.client.SendTelemetry(ctx, platformclient.NewIdempotencyKey(fmt.Sprintf("node-%d", p.nodeID)), platformclient.TelemetryBatch{
		Events: []platformclient.TelemetryEvent{{
			Type: "node.telemetry", NodeID: p.nodeID, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
			Data: map[string]any{
				"revision": p.revision.Load(),
				"traffic":  payload.Traffic, "alive": payload.Alive, "online": payload.Online,
				"cpu": payload.CPU, "mem": payload.Mem, "swap": payload.Swap, "disk": payload.Disk,
				"metrics": payload.Metrics,
			},
		}},
	})
}

func (p *DevicePlatformControlPlane) ReportDevices(push PushClient, devices map[int][]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := p.client.SendTelemetry(ctx, platformclient.NewIdempotencyKey(fmt.Sprintf("devices-%d", p.nodeID)), platformclient.TelemetryBatch{
		Events: []platformclient.TelemetryEvent{{
			Type: "node.devices", NodeID: p.nodeID, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
			Data: map[string]any{"devices": devices},
		}},
	})
	if err != nil {
		nlog.Core().Warn("device platform device report failed", "node_id", p.nodeID, "error", err)
	}
}

func (p *DevicePlatformControlPlane) Metrics() APIMetrics {
	success, failure := p.client.Metrics()
	return APIMetrics{Success: success, Failure: failure}
}
