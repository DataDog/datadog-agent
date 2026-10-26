// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && !windows

package phonehome

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/stretchr/testify/require"
)

func TestSupervisorReplacementRequiresBothNewIDsAndRC(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	require.Equal(t, pollInterval, f.c.step(ctx))
	require.Equal(t, 1, f.pm.starts)
	f.rc.onRead = func() { t.Fatal("same supervisor lifecycle must not recheck RC") }
	f.pm.control.State = pb.ProcessState_FAILED
	f.c.step(ctx)
	require.Equal(t, 1, f.pm.starts, "process crash recovery belongs to the supervisor")
	f.pm.describeErr = errors.New("supervisor disconnected")
	require.Equal(t, pollInterval, f.c.step(ctx))
	require.Equal(t, "supervisor_unavailable", f.state.Reason)
	f.pm.describeErr = nil
	f.c.step(ctx)
	require.Equal(t, "supervisor_start_acknowledged", f.state.Reason)

	f.pm.control.Uuid = "new-control"
	f.pm.control.State = pb.ProcessState_SKIPPED
	f.c.step(ctx)
	require.Equal(t, "supervisor_registration_change_pending", f.state.Reason)
	require.Equal(t, 1, f.pm.starts)
	f.pm.executor.Uuid = "new-executor"
	f.rc.onRead = nil
	f.rc.rc.Authorized = false
	f.c.step(ctx)
	require.Equal(t, "rc_key_unauthorized", f.state.Reason)
	require.Equal(t, 1, f.pm.starts)
	f.rc.rc.Authorized = true
	f.c.step(ctx)
	require.Equal(t, 2, f.pm.starts)
	f.c.step(ctx)
	require.Equal(t, 2, f.pm.starts)
}

func TestAmbiguousStartIsNotRetriedAfterSupervisorReplacement(t *testing.T) {
	f := newFixture(t)
	f.pm.startErr = errors.New("response lost")
	require.Zero(t, f.c.step(context.Background()))
	require.Equal(t, "launch_outcome_unknown_requires_review", f.state.Reason)
	f.pm.startErr = nil
	f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
	f.rc.onRead = func() { t.Fatal("ambiguous handoff must not re-arm") }
	require.Zero(t, f.c.step(context.Background()))
	require.Equal(t, 1, f.pm.starts)
}

func TestExistingChildrenAreWatchedWithoutTakingOverTheirLifecycle(t *testing.T) {
	f := newFixture(t)
	f.pm.control.State = pb.ProcessState_RUNNING
	f.c.step(context.Background())
	require.Zero(t, f.pm.starts)
	require.Equal(t, "existing_process_left_untouched", f.state.Reason)
	f.pm.control.State = pb.ProcessState_STOPPED
	f.c.step(context.Background())
	require.Zero(t, f.pm.starts, "a stopped child is not a new registration")
	f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
	f.c.step(context.Background())
	require.Equal(t, 1, f.pm.starts)
}

func TestSupervisorWatchLoopRecoversAndCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() { defer close(done); f.c.run(ctx) }()
		time.Sleep(pollInterval)
		synctest.Wait()
		require.Equal(t, 1, f.pm.starts)
		f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
		time.Sleep(pollInterval)
		synctest.Wait()
		require.Equal(t, 2, f.pm.starts)
		cancel()
		<-done
	})
}

func TestHTTPStackSupervisorReplacementRequiresFreshScope(t *testing.T) {
	f := newScopeFixture(t)
	c, pm := stackController(t, f)
	ctx := context.Background()
	f.rc(true, true)
	c.step(ctx)
	f.step()
	c.step(ctx)
	require.Equal(t, 1, pm.starts)
	require.False(t, f.s.demand.Load())
	f.advance(time.Minute)
	f.step()
	require.EqualValues(t, 1, f.aaaGETs.Load(), "watching IDs must not keep AAA polling")

	pm.control.Uuid, pm.executor.Uuid = "new-control", "new-executor"
	f.rc(false, true)
	c.step(ctx)
	f.step()
	require.EqualValues(t, 1, f.aaaGETs.Load())
	require.Equal(t, 1, pm.starts)
	f.rc(true, true)
	c.step(ctx)
	require.Equal(t, 1, pm.starts, "previous supervisor's scope grant must not be reused")
	f.step()
	c.step(ctx)
	require.EqualValues(t, 2, f.aaaGETs.Load())
	require.Equal(t, 2, pm.starts)
	require.False(t, f.s.demand.Load())
}
