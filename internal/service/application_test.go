package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ClaraCora/CPanelde/internal/config"
	"github.com/ClaraCora/CPanelde/internal/controlplane"
	"github.com/ClaraCora/CPanelde/internal/model"
)

type applicationTestPlane struct {
	reportTestControlPlane
	bootstrap controlplane.Bootstrap
	results   []controlplane.ApplicationResult
}

func (p *applicationTestPlane) Initial(context.Context, func() map[string]interface{}, chan<- controlplane.Event, chan<- controlplane.StatusChange) (controlplane.Bootstrap, error) {
	return p.bootstrap, nil
}
func (p *applicationTestPlane) ReportApplication(result controlplane.ApplicationResult) {
	p.results = append(p.results, result)
}

func TestInitialApplicationAcknowledgedOnlyAfterKernelSuccess(t *testing.T) {
	for _, test := range []struct {
		name       string
		config     *model.NodeSpec
		users      []model.UserSpec
		startErr   error
		wantStage  string
		wantStatus string
	}{
		{name: "success", config: &model.NodeSpec{Protocol: "vless", ServerPort: 443}, users: []model.UserSpec{{ID: 1, UUID: "user"}}, wantStage: "kernel", wantStatus: "applied"},
		{name: "validation failure", config: &model.NodeSpec{Protocol: "unsupported"}, users: []model.UserSpec{{ID: 1, UUID: "user"}}, wantStage: "validate", wantStatus: "failed"},
		{name: "kernel failure", config: &model.NodeSpec{Protocol: "vless", ServerPort: 443}, users: []model.UserSpec{{ID: 1, UUID: "user"}}, startErr: errors.New("bind refused"), wantStage: "kernel", wantStatus: "failed"},
		{name: "no users pending", config: &model.NodeSpec{Protocol: "vless", ServerPort: 443}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cp := &applicationTestPlane{bootstrap: controlplane.Bootstrap{Revision: 7, Config: test.config, Users: test.users}}
			s := newTestService(&fakeKernel{startErr: test.startErr})
			s.cfg = &config.Config{Kernel: config.KernelConfig{Type: "xray"}}
			s.source = cp
			err := s.initialSetup(context.Background())
			if test.wantStatus == "failed" && err == nil {
				t.Fatal("expected startup failure")
			}
			if test.wantStatus != "failed" && err != nil {
				t.Fatal(err)
			}
			if test.wantStatus == "" {
				if len(cp.results) != 0 {
					t.Fatalf("idle config was claimed applied: %+v", cp.results)
				}
				return
			}
			if len(cp.results) != 1 || cp.results[0].Revision != 7 || cp.results[0].Status != test.wantStatus || cp.results[0].Stage != test.wantStage {
				t.Fatalf("results=%+v", cp.results)
			}
		})
	}
}

func TestFailedNewRevisionRetainsBaselineAndReportsFailure(t *testing.T) {
	for _, stage := range []string{"validate", "kernel"} {
		t.Run(stage, func(t *testing.T) {
			cp := &applicationTestPlane{}
			k := &fakeKernel{running: true}
			if stage == "kernel" {
				k.reloadErr = errors.New("new inbound failed")
			}
			s := newTestService(k)
			s.cfg = &config.Config{Kernel: config.KernelConfig{Type: "xray"}}
			s.source = cp
			old := &model.NodeSpec{Protocol: "vless", ServerPort: 443}
			s.lastConfig = old
			s.lastConfigHash = computeConfigHash(old)
			oldUsers := []model.UserSpec{{ID: 1, UUID: "old"}}
			s.updateUserState(oldUsers)
			s.appliedState.Config = old
			s.appliedState.Users = oldUsers
			s.desiredRevision = 1
			s.appliedRevision = 1
			next := &model.NodeSpec{Protocol: "vless", ServerPort: 8443}
			if stage == "validate" {
				next.Protocol = "unsupported"
			}
			users := []model.UserSpec{{ID: 2, UUID: "new"}}
			s.applyPullResult(context.Background(), pullResult{revision: 2, config: next, configHash: computeConfigHash(next), users: users, userHash: computeUserHash(users)})
			if s.lastConfig != old || s.lastConfigHash != computeConfigHash(old) || s.lastUsers[0].UUID != "old" || s.appliedRevision != 1 || s.desiredRevision != 1 {
				t.Fatalf("failed revision replaced baseline: %+v", s)
			}
			if len(cp.results) != 1 || cp.results[0].Revision != 2 || cp.results[0].Status != "failed" || cp.results[0].Stage != stage {
				t.Fatalf("results=%+v", cp.results)
			}
			if !k.running {
				t.Fatal("old kernel stopped after failed replacement")
			}
			if !s.reconcileNeeded.Load() {
				t.Fatal("failed application will not be retried while websocket is connected")
			}
			if stage == "kernel" && (k.startCalls != 1 || len(k.startUsers) != 1 || k.startUsers[0].UUID != "old") {
				t.Fatal("last good runtime was not restored after partial reload")
			}
			// The unchanged failed config must be retried, then acknowledged only
			// after the kernel accepts it.
			if stage == "kernel" {
				k.reloadErr = nil
				s.applyPullResult(context.Background(), pullResult{revision: 2, config: next, configHash: computeConfigHash(next), users: users, userHash: computeUserHash(users)})
				if s.appliedRevision != 2 || len(cp.results) != 2 || cp.results[1].Status != "applied" {
					t.Fatalf("retry did not apply: %+v", cp.results)
				}
				if s.reconcileNeeded.Load() {
					t.Fatal("successful retry did not clear reconciliation state")
				}
			}
		})
	}
}

func TestEquivalentConfigRevisionCanBeConfirmedWithoutRestart(t *testing.T) {
	cp := &applicationTestPlane{}
	k := &fakeKernel{running: true}
	s := newTestService(k)
	s.source = cp
	s.lastConfig = &model.NodeSpec{Protocol: "vless", ServerPort: 443}
	s.updateUserState([]model.UserSpec{{ID: 1, UUID: "old"}})
	s.desiredRevision = 1
	s.appliedRevision = 1
	s.applyPullResult(context.Background(), pullResult{revision: 2, users: s.lastUsers, userHash: s.lastUserHash})
	if s.appliedRevision != 2 || k.reloadCalls != 0 || len(cp.results) != 1 || cp.results[0].Status != "applied" {
		t.Fatalf("unchanged runtime result=%+v", cp.results)
	}
}

func TestIdleNodeAcknowledgesOnlyWhenFirstUsersStartKernel(t *testing.T) {
	cp := &applicationTestPlane{bootstrap: controlplane.Bootstrap{Revision: 9, Config: &model.NodeSpec{Protocol: "vless", ServerPort: 443}}}
	k := &fakeKernel{}
	s := newTestService(k)
	s.cfg = &config.Config{Kernel: config.KernelConfig{Type: "xray"}}
	s.source = cp
	if err := s.initialSetup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cp.results) != 0 {
		t.Fatal("idle node was prematurely acknowledged")
	}
	users := []model.UserSpec{{ID: 1, UUID: "first"}}
	s.applyUserUpdate(context.Background(), users, computeUserHash(users))
	if !k.running || s.appliedRevision != 9 || len(cp.results) != 1 || cp.results[0].Status != "applied" {
		t.Fatalf("first runtime did not confirm application: %+v", cp.results)
	}
}
