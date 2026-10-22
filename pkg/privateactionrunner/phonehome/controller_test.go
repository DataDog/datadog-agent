// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && !windows

package phonehome

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type supervisor struct {
	pb.ProcessManagerClient
	control, executor *pb.ProcessDetail
	starts            int
	startErr          error
}

func (p *supervisor) Describe(_ context.Context, r *pb.DescribeRequest, _ ...grpc.CallOption) (*pb.DescribeResponse, error) {
	if r.NameOrUuid == controlProcess {
		return &pb.DescribeResponse{Detail: p.control}, nil
	}
	return &pb.DescribeResponse{Detail: p.executor}, nil
}
func (p *supervisor) Start(_ context.Context, _ *pb.StartRequest, _ ...grpc.CallOption) (*pb.StartResponse, error) {
	p.starts++
	return &pb.StartResponse{}, p.startErr
}
func (*supervisor) Stop(context.Context, *pb.StopRequest, ...grpc.CallOption) (*pb.StopResponse, error) {
	panic("startup gating must never stop an existing process or credential holder")
}

type fixture struct {
	c     *controller
	pm    *supervisor
	rc    *statusService
	cfg   model.BuildableConfig
	state State
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	for k, v := range map[string]interface{}{"api_key": "key", "site": "datadoghq.com", "dd_url": "", "remote_configuration.enabled": true, par.Enabled: true, par.IdentityFilePath: filepath.Join(t.TempDir(), "identity.json")} {
		cfg.SetInTest(k, v)
	}
	f := &fixture{cfg: cfg, rc: &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}, pm: &supervisor{
		control:  &pb.ProcessDetail{Uuid: "control", State: pb.ProcessState_SKIPPED, RestartPolicy: "never"},
		executor: &pb.ProcessDetail{Uuid: "executor", State: pb.ProcessState_SKIPPED, RestartPolicy: "never"},
	}}
	f.c = &controller{cfg: cfg, source: NewSource(cfg, f.rc), pm: f.pm, now: time.Now, emit: func(s State) { f.state = s }}
	return f
}

// This uses the production RC-only source, not an injected PAR-scope grant.
func TestRCOnlyStartupProductionSource(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%t_%t", enabled, authorized), func(t *testing.T) {
				f := newFixture(t)
				f.rc.rc = nil
				f.c.step(context.Background())
				require.Zero(t, f.pm.starts)
				f.rc.rc = &pbgo.OrgStatusResponse{Enabled: enabled, Authorized: authorized}
				f.c.step(context.Background())
				if enabled && authorized {
					require.Equal(t, 1, f.pm.starts)
				} else {
					require.Zero(t, f.pm.starts)
					f.rc.rc = &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}
					f.c.step(context.Background())
					require.Equal(t, 1, f.pm.starts, "later RC enablement needs no restart")
				}
				require.Equal(t, "supervisor_start_acknowledged", f.state.Reason)
				require.NoFileExists(t, f.cfg.GetString(par.IdentityFilePath), "the gate does not enroll")
				f.c.step(context.Background())
				require.Equal(t, 1, f.pm.starts)
			})
		}
	}
}

func TestStartupDoesNotReplayOrStopChildren(t *testing.T) {
	for _, state := range []pb.ProcessState{pb.ProcessState_RUNNING, pb.ProcessState_STARTING, pb.ProcessState_STOPPING, pb.ProcessState_FAILED, pb.ProcessState_EXITED, pb.ProcessState_CRASHED} {
		for _, executor := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/executor=%t", state, executor), func(t *testing.T) {
				f := newFixture(t)
				if executor {
					f.pm.executor.State = state
				} else {
					f.pm.control.State = state
				}
				require.Zero(t, f.c.step(context.Background()))
				require.Zero(t, f.pm.starts)
			})
		}
	}
	f := newFixture(t)
	f.pm.startErr = errors.New("response lost")
	require.Zero(t, f.c.step(context.Background()))
	require.Equal(t, "launch_outcome_unknown_requires_review", f.state.Reason)
	f.c.step(context.Background())
	require.Equal(t, 1, f.pm.starts)
}

func TestStartupRechecksRCAndCancellation(t *testing.T) {
	f := newFixture(t)
	reads := 0
	f.rc.onRead = func() {
		reads++
		if reads == 2 {
			f.rc.rc.Authorized = false
		}
	}
	f.c.step(context.Background())
	require.Zero(t, f.pm.starts)
	require.Equal(t, "rc_key_unauthorized", f.state.Reason)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Zero(t, f.c.step(ctx))
}

type deadlineSupervisor struct {
	*supervisor
	startContextErr error
}

func (p *deadlineSupervisor) Start(ctx context.Context, r *pb.StartRequest, opts ...grpc.CallOption) (*pb.StartResponse, error) {
	p.startContextErr = ctx.Err()
	if p.startContextErr != nil {
		return nil, p.startContextErr
	}
	return p.supervisor.Start(ctx, r, opts...)
}

func TestStartGetsFreshDeadlineAfterPreflight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		pm := &deadlineSupervisor{supervisor: f.pm}
		f.c.pm = pm
		f.rc.onRead = func() {
			f.rc.onRead = nil
			// Model a scheduling pause after successful supervisor inspection.
			time.Sleep(6 * time.Second)
		}
		require.Zero(t, f.c.step(context.Background()))
		require.NoError(t, pm.startContextErr, "Start must not inherit an exhausted preflight deadline")
		require.Equal(t, 1, pm.starts)
		require.Equal(t, "supervisor_start_acknowledged", f.state.Reason)
		f.c.step(context.Background())
		require.Equal(t, 1, pm.starts)
	})
}

func TestStartupPreservesLegacyRecoveryAndRequiresOnDemandSupervisor(t *testing.T) {
	f := newFixture(t)
	journal := f.cfg.GetString(par.IdentityFilePath) + ".phone-home"
	require.NoError(t, os.Mkdir(journal, 0700))
	f.c.step(context.Background())
	require.Equal(t, "legacy_attempt_requires_reconciliation", f.state.Reason)
	require.DirExists(t, journal)
	require.Zero(t, f.pm.starts)
	f = newFixture(t)
	f.pm.control.AutoStart = true
	f.c.step(context.Background())
	require.Equal(t, "supervisor_must_be_on_demand_no_restart", f.state.Reason)
	require.Zero(t, f.pm.starts)
}
