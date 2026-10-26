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

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
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
	require.Equal(t, phonehomedef.ReasonSupervisorUnavailable, f.state.Reason)
	f.pm.describeErr = nil
	f.c.step(ctx)
	require.Equal(t, phonehomedef.ReasonSupervisorStartAcknowledged, f.state.Reason)

	f.pm.control.Uuid = "new-control"
	f.pm.control.State = pb.ProcessState_SKIPPED
	f.c.step(ctx)
	require.Equal(t, phonehomedef.ReasonSupervisorRegistrationChangePending, f.state.Reason)
	require.Equal(t, 1, f.pm.starts)
	f.pm.executor.Uuid = "new-executor"
	f.rc.onRead = nil
	f.rc.rc.Authorized = false
	f.c.step(ctx)
	require.Equal(t, phonehomedef.ReasonRCKeyUnauthorized, f.state.Reason)
	require.Equal(t, 1, f.pm.starts)
	f.rc.rc.Authorized = true
	f.c.step(ctx)
	require.Equal(t, 2, f.pm.starts)
	f.c.step(ctx)
	require.Equal(t, 2, f.pm.starts)
}

func TestInvalidConfigInHandedOffLifecycleDoesNotStopWatching(t *testing.T) {
	f := newFixture(t)
	f.c.step(context.Background())
	f.pm.control.State = pb.ProcessState_INVALID_CONFIG
	f.pm.control.RestartPolicy = ""
	require.Equal(t, pollInterval, f.c.step(context.Background()))
	require.Equal(t, phonehomedef.StateHandedOff, f.state.State)
	require.Equal(t, 1, f.pm.starts)
	f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
	f.pm.control.State = pb.ProcessState_SKIPPED
	f.pm.control.RestartPolicy = "never"
	f.c.step(context.Background())
	require.Equal(t, 2, f.pm.starts)
}

func TestAmbiguousStartIsNotRetriedAfterSupervisorReplacement(t *testing.T) {
	f := newFixture(t)
	f.pm.startErr = errors.New("response lost")
	require.Zero(t, f.c.step(context.Background()))
	require.Equal(t, phonehomedef.ReasonLaunchOutcomeUnknownRequiresReview, f.state.Reason)
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
	require.Equal(t, phonehomedef.ReasonExistingProcessLeftUntouched, f.state.Reason)
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
		ctx, cancel := context.WithTimeout(context.Background(), 3*pollInterval)
		defer cancel()
		f.c.emit = func(s State) {
			if s.State == phonehomedef.StateHandedOff {
				if f.pm.starts == 1 {
					f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
				} else {
					cancel()
				}
			}
		}
		f.c.run(ctx)
		require.Equal(t, 2, f.pm.starts)
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})
}

func TestMissingRegistrationIDDoesNotIssueEmptyLookup(t *testing.T) {
	for _, control := range []bool{false, true} {
		f := newFixture(t)
		if control {
			f.pm.control.Uuid = ""
		} else {
			f.pm.executor.Uuid = ""
		}
		f.pm.onDescribe = func(r *pb.DescribeRequest) { require.NotEmpty(t, r.NameOrUuid) }
		require.Equal(t, pollInterval, f.c.step(context.Background()))
		require.Equal(t, phonehomedef.ReasonSupervisorRegistrationUnavailable, f.state.Reason)
		require.Zero(t, f.pm.starts)
	}
}

func TestRestartBetweenDescribeCallsDoesNotCaptureMixedIDs(t *testing.T) {
	for _, active := range []bool{false, true} {
		f := newFixture(t)
		if active {
			f.pm.control.State = pb.ProcessState_RUNNING
		}
		f.pm.onDescribe = func(r *pb.DescribeRequest) {
			if r.NameOrUuid == executorProcess {
				f.pm.onDescribe = nil
				f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
				f.pm.control.State = pb.ProcessState_SKIPPED
			}
		}
		f.c.step(context.Background())
		require.Equal(t, phonehomedef.ReasonSupervisorRegistrationChangePending, f.state.Reason)
		require.Empty(t, f.c.generation)
		require.Zero(t, f.pm.starts)
		f.c.step(context.Background())
		require.Equal(t, 1, f.pm.starts)
	}
}

func TestMissingRegistrationAtStartCanBeReobserved(t *testing.T) {
	f := newFixture(t)
	f.pm.startErr = grpcstatus.Error(codes.NotFound, "registration disappeared before spawn")
	require.Equal(t, pollInterval, f.c.step(context.Background()))
	require.False(t, f.c.attempted)
	f.pm.startErr = nil
	f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
	f.c.step(context.Background())
	require.Equal(t, 2, f.pm.starts)
	require.Equal(t, phonehomedef.ReasonSupervisorStartAcknowledged, f.state.Reason)
}

func TestAmbiguousStartAfterRearmingStopsRecovery(t *testing.T) {
	f := newFixture(t)
	f.c.step(context.Background())
	f.pm.control.Uuid, f.pm.executor.Uuid = "new-control", "new-executor"
	f.pm.startErr = errors.New("response lost")
	require.Zero(t, f.c.step(context.Background()))
	f.pm.control.Uuid, f.pm.executor.Uuid = "another-control", "another-executor"
	f.pm.startErr = nil
	require.Zero(t, f.c.step(context.Background()))
	require.Equal(t, 2, f.pm.starts)
}

type forwardingSource struct{ Source }

func TestExistingChildrenReleaseDiscoveryThroughSourceInterface(t *testing.T) {
	f := newScopeFixture(t)
	c, pm := stackController(t, f)
	c.source = forwardingSource{c.source}
	f.rc(true, true)
	c.step(context.Background())
	f.step()
	require.True(t, f.s.demand.Load())
	pm.control.State = pb.ProcessState_RUNNING
	c.step(context.Background())
	require.False(t, f.s.demand.Load())
	require.Zero(t, pm.starts)
	f.advance(time.Minute)
	f.step()
	require.EqualValues(t, 1, f.aaaGETs.Load())
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
